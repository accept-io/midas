// /explorer/assets/js/graph/authority/authority-cytoscape-poc.js
//
// Cytoscape.js Authority Graph PoC. Strategic evaluation prototype —
// NOT production code. Self-contained and deletable: a single file
// plus its CSS, plus a one-line script tag in index.html.
//
// Toggle:
//   • Append ?cytoscape=1 to the Explorer URL.
//     e.g. http://localhost:8080/explorer?cytoscape=1#services
//   • Without the flag this module loads but does nothing — the
//     existing Authority Graph view continues to own the render path
//     byte-identically.
//
// Mechanism:
//   • When the flag is present, the PoC registers itself as the
//     'authority' lens implementation in the renderer dispatch table.
//     `register('authority', impl)` overwrites the prior registration;
//     because this module loads AFTER authority-graph-view.js the PoC
//     wins. Removing the script tag from index.html restores the
//     original view with zero residual state.
//   • Existing fetch path (shell.refresh) flows the projection payload
//     through to the PoC's render(payload, mount) hook unchanged.
//
// What this PoC demonstrates:
//   • Live Authority Graph data driving a Cytoscape canvas.
//   • Deterministic vertical-lane layout (preset positions).
//   • Hover behaviours: highlight focused node + its incident edges +
//     directly-connected neighbours; dim everything else.
//   • Edge hover: surface the relationship by emphasising endpoints.
//   • Click: select node, render a minimal PoC inspector panel.
//   • Background tap: clear selection / restore baseline.
//   • Path emphasis: walk predecessors back to the Business Service
//     root for any clicked spine node.
//   • Node dragging with auto-routed edges (Cytoscape handles this
//     natively — zero code required).
//   • Workbench legend overlay: kind colours + future-overlay
//     placeholders (drift, resilience, diagnostics, runtime evidence).
//
// What this PoC deliberately does NOT solve:
//   • Inspector integration with the existing #gmap-details right
//     drawer. PoC shows its own inline panel.
//   • Authority Workbench bottom letterbox.
//   • Layer-state filtering (fail-mode toggle etc.).
//   • Drift/resilience/diagnostics/runtime overlay rendering — only
//     legend placeholders for now.
//   • Visual semantics convergence with D32h-fix-2e tokens.

(function () {
  'use strict';

  // ── Activation gate ──────────────────────────────────────────────────

  function _isPocActive() {
    try {
      var sp = new URLSearchParams(window.location.search);
      return sp.get('cytoscape') === '1';
    } catch (_) {
      return false;
    }
  }

  if (!_isPocActive()) {
    // No-op: original Authority view's lens registration stands.
    return;
  }

  // ── D33a-spike-1 — Theme exploration (Cytoscape-native only) ─────────
  //
  // `?cyTheme=` selects which Cytoscape node styling variant to apply.
  // Unknown / missing values fall back to 'classic' (the current PoC
  // baseline). All themes share the same mapping, layout, and
  // interaction wiring — only the Cytoscape `style` array differs.
  //
  // No HTML node overlays. No external image assets. Object-card and
  // object-tile-v3 icons come from the MIDAS icon registry (Lucide,
  // vendored under assets/icons/lucide/) via _iconForKind() below.
  // D33a-spike-2 — Theme list expanded with four richer themes:
  //   • object-card-v2 — Cytoscape-native pushed to the limit
  //   • glass-card     — translucent layered card aesthetic
  //   • holo-card      — luminous accent treatment
  //   • html-card      — DOM overlay above the Cytoscape canvas
  // The three D33a-spike-1 themes remain unchanged at the head of the
  // list so existing query URLs and contract pins continue to work.
  // D33a-spike-2g-impl-4 — `authority-thin-card-v1` appended at the
  // tail. Existing themes stay at their established indices so all
  // prior contract pins continue to hold.
  var _THEMES        = ['classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card', 'object-tile-v3', 'authority-thin-card-v1'];
  var DEFAULT_THEME  = 'classic';

  function _resolveTheme() {
    try {
      var sp = new URLSearchParams(window.location.search);
      var raw = (sp.get('cyTheme') || '').trim();
      if (_THEMES.indexOf(raw) >= 0) return raw;
      return DEFAULT_THEME;
    } catch (_) {
      return DEFAULT_THEME;
    }
  }
  var _activeTheme = _resolveTheme();

  // Mark <body> so CSS can hide #gmap-canvas and reveal the PoC mount.
  if (document.body) {
    document.body.classList.add('cytoscape-poc-active');
  } else {
    document.addEventListener('DOMContentLoaded', function () {
      document.body.classList.add('cytoscape-poc-active');
    });
  }

  // ── Token resolution (use existing design tokens) ────────────────────

  function _token(name, fallback) {
    try {
      var v = getComputedStyle(document.documentElement)
        .getPropertyValue(name);
      return (v && v.trim()) || fallback;
    } catch (_) {
      return fallback;
    }
  }

  // Build the token palette once on first render. Re-resolving each
  // render is unnecessary — operators don't theme-flip in-flight.
  var _palette = null;
  function _resolvePalette() {
    if (_palette) return _palette;
    _palette = {
      surfaceLow:   _token('--surface-container-low',   '#1a1f2e'),
      surface:      _token('--surface-container',       '#222838'),
      onSurface:    _token('--on-surface',              '#e2e8f0'),
      onSurfaceMut: _token('--on-surface-variant',      '#94a3b8'),
      outline:      _token('--outline-variant',         '#475569'),
      primary:      _token('--primary',                 '#7aa2ff'),
      slate300:     _token('--slate-300',               '#cbd5e1'),
      slate400:     _token('--slate-400',               '#94a3b8'),
      badgeGood:    _token('--badge-good',              '#1d6b3f'),
      badgeWarn:    _token('--badge-warn',              '#8a601f'),
      badgeBad:     _token('--badge-bad',               '#8a1f1f'),
      badgeInfo:    _token('--badge-info',              '#1f4f8a'),
    };
    return _palette;
  }

  // Per-kind visual descriptors. Mirrors the Authority Graph design
  // specification §4.1 category-colour scheme but tuned for the PoC's
  // workbench-grade palette (not garish).
  function _kindStyle(palette) {
    return {
      business_service: { fill: palette.primary,     stroke: palette.primary,   shape: 'round-rectangle', size: { w: 220, h: 64 } },
      decision_surface: { fill: palette.badgeInfo,   stroke: palette.badgeInfo, shape: 'round-rectangle', size: { w: 200, h: 56 } },
      authority_profile:{ fill: palette.badgeGood,   stroke: palette.badgeGood, shape: 'round-rectangle', size: { w: 200, h: 56 } },
      authority_grant:  { fill: palette.badgeWarn,   stroke: palette.badgeWarn, shape: 'round-rectangle', size: { w: 200, h: 56 } },
      agent:            { fill: palette.onSurface,   stroke: palette.slate300,  shape: 'round-rectangle', size: { w: 200, h: 56 } },
      fail_mode_policy: { fill: palette.slate400,    stroke: palette.slate400,  shape: 'round-rectangle', size: { w: 180, h: 48 } },
      escalation_target:{ fill: palette.slate400,    stroke: palette.slate400,  shape: 'round-rectangle', size: { w: 180, h: 48 } },
    };
  }

  // ── D33a-spike-2g-impl-3 — Registry-backed Lucide icons ──────────────
  //
  // Authority node kinds are mapped to MIDAS-facing icon keys exposed by
  // the icon registry (assets/js/icons/midas-icons.js, attached at
  // window.MIDASExplorerIcons). The registry resolves each key to a
  // vendored Lucide SVG and produces a Cytoscape-ready data URI via
  // cytoscapeDataURI(). The registry script is loaded before this
  // module by index.html, so window.MIDASExplorerIcons is available at
  // module-load time when the PoC gate is open.
  //
  // The previous PoC carried a self-authored data URI map here. That
  // map has been replaced (D33a-spike-2g-impl-3) so all PoC icons now
  // come from the curated 30-icon Lucide subset vendored in
  // D33a-spike-2g-impl-1 under assets/icons/lucide/.
  //
  // No Lucide filename appears in this file: the PoC speaks only in
  // MIDAS-facing keys. The registry owns the filename mapping.
  var _AUTHORITY_KIND_ICON_KEYS = {
    business_service:  'authorityBusinessService',
    decision_surface:  'authorityDecisionSurface',
    authority_profile: 'authorityProfile',
    authority_grant:   'authorityGrant',
    agent:             'authorityAgent',
    fail_mode_policy:  'authorityFailModePolicy',
    escalation_target: 'authorityEscalationTarget',
  };

  // _iconForKind returns a Cytoscape data: URI for the given Authority
  // node kind, or '' if the registry is unavailable or the key is
  // unknown. The stroke colour defaults to '#e2e8f0' (the same neutral
  // high-contrast token the previous self-authored icons used), so the
  // visual identity of object-card / object-card-v2 / object-tile-v3
  // is preserved across this swap. Callers may override the stroke
  // when a theme has a kind-specific palette colour with adequate
  // contrast against the node fill.
  //
  // Safe-fallback contract: never throws. Returns '' if
  //   • window.MIDASExplorerIcons is missing (registry didn't load);
  //   • icons.has is not a function (registry shape is wrong);
  //   • the kind has no mapping;
  //   • icons.has(key) returns false (key absent in registry).
  // Cytoscape treats '' as "no background image" so a missing registry
  // degrades to icon-less nodes rather than a render error.
  function _iconForKind(kind, stroke) {
    var icons = window.MIDASExplorerIcons;
    var key = _AUTHORITY_KIND_ICON_KEYS[kind];
    if (!icons || !key || typeof icons.has !== 'function' || !icons.has(key)) {
      return '';
    }
    return icons.cytoscapeDataURI(key, { stroke: stroke || '#e2e8f0' });
  }

  // ── D33a-spike-2 — Display helpers for rich themes ───────────────────
  //
  // _nodeTypeLabel returns the operator-facing display name for a kind.
  // Used by the rich themes to compose a two-line label (Title / TYPE)
  // and by the HTML overlay to render the type chip.
  function _nodeTypeLabel(kind) {
    switch (kind) {
      case 'business_service':  return 'Business Service';
      case 'decision_surface':  return 'Decision Surface';
      case 'authority_profile': return 'Authority Profile';
      case 'authority_grant':   return 'Authority Grant';
      case 'agent':             return 'Agent';
      case 'fail_mode_policy':  return 'Fail-Mode Policy';
      case 'escalation_target': return 'Escalation Target';
      default:                  return String(kind || '').replace(/_/g, ' ');
    }
  }

  // _displayLabel truncates a node label for in-card display so long
  // ids don't overflow the card boundary. The full label remains
  // available in `node.data().label` for the inspector / HTML overlay.
  // Accepts either a Cytoscape node ref (with `.data(...)` accessor)
  // or a raw mapper entry ({ data: { label, id } }).
  function _displayLabel(ele, maxLen) {
    if (!ele) return '';
    var lbl = '';
    if (typeof ele.data === 'function') {
      lbl = String(ele.data('label') || ele.data('id') || '');
    } else {
      var d = ele.data || ele;
      lbl = String(d.label || d.id || '');
    }
    maxLen = (typeof maxLen === 'number' && maxLen > 4) ? maxLen : 38;
    if (lbl.length > maxLen) return lbl.slice(0, maxLen - 1) + '…';
    return lbl;
  }

  // ── D33a-spike-2g-impl-4 — Title normalisation for thin card ─────────
  //
  // _displayTitle normalises demo / showcase prefixes off node labels
  // so the native thin card carries a concise readable title instead
  // of long fixture ids. The full label remains in `node.data().label`
  // for the inspector — only the visible card text is normalised.
  //
  // Rules (applied in order):
  //   • "Showcase: <name>"               → "<name>"
  //   • "Showcase <name>"                → "<name>"
  //   • "Demo <name>"                    → "<name>"
  //   • "grant-demo-<rest>"              → "<rest>"
  //   • " FailModePolicy" / "FailModePolicy" suffix → " Policy" / "Policy"
  //
  // After normalisation, the title is clamped to maxLen (default 32)
  // and an ellipsis is appended if it overflows. We never split the
  // title across multiple lines in the native card — that belongs to
  // a future HTML overlay or workbench-chrome card.
  function _displayTitle(ele, maxLen) {
    if (!ele) return '';
    var raw = '';
    if (typeof ele.data === 'function') {
      raw = String(ele.data('label') || ele.data('id') || '');
    } else {
      var d = ele.data || ele;
      raw = String(d.label || d.id || '');
    }
    var s = raw;
    // Showcase / Demo / grant-demo prefix stripping.
    s = s.replace(/^Showcase:\s*/i, '');
    s = s.replace(/^Showcase\s+/i, '');
    s = s.replace(/^Demo\s+/i, '');
    s = s.replace(/^grant-demo-/i, '');
    // FailModePolicy collapse — "Closed-Only FailModePolicy" → "Closed-Only Policy".
    s = s.replace(/\s*FailModePolicy\b/g, ' Policy');
    s = s.replace(/^\s+|\s+$/g, '');
    if (!s) s = raw;
    maxLen = (typeof maxLen === 'number' && maxLen > 4) ? maxLen : 32;
    if (s.length > maxLen) return s.slice(0, maxLen - 1) + '…';
    return s;
  }

  // ── D33a-spike-2g-impl-4a — Two-line card subtitle vocabulary ────────
  //
  // The thin card carries a native two-line label "Title / Subtitle".
  // The subtitle is the operator-facing kind name — deterministic,
  // controlled vocabulary, no runtime state (no diagnostics counts,
  // no posture flags, no risk score). Unknown kinds resolve to an
  // empty subtitle so the helper degrades to a single-line title.
  //
  // Cytoscape labels are single-style — both lines share one
  // font-size / font-weight / colour. This is a deliberate native
  // approximation of the Palantir Title+Subtitle composition; a
  // future HTML overlay tranche can render mixed typography if
  // needed.
  var _NODE_SUBTITLES = {
    business_service:  'Business Service',
    decision_surface:  'Decision Surface',
    authority_profile: 'Authority Profile',
    authority_grant:   'Authority Grant',
    agent:             'Agent',
    fail_mode_policy:  'Fail-Mode Policy',
    escalation_target: 'Escalation Target',
  };

  function _nodeSubtitle(ele) {
    if (!ele) return '';
    var kind = '';
    if (typeof ele.data === 'function') {
      kind = String(ele.data('kind') || '');
    } else {
      var d = ele.data || ele;
      kind = String(d.kind || '');
    }
    return _NODE_SUBTITLES[kind] || '';
  }

  // _displayCardLabel composes the native two-line thin-card label:
  //
  //   <normalised title>
  //   <kind subtitle>
  //
  // Falls back to title-only if the subtitle resolves to empty. The
  // helper is the only label binding the thin-card branch uses.
  function _displayCardLabel(ele, maxTitleLen) {
    var title = _displayTitle(ele, maxTitleLen);
    var subtitle = _nodeSubtitle(ele);
    if (!subtitle) return title;
    return title + '\n' + subtitle;
  }

  // ── D33a-spike-2g-impl-4c — Strategic symbol vocabulary + helper ─────
  //
  // Authority card carries up to TWO right-side strategic symbols that
  // communicate semantic state (severity / blocked / suspended /
  // override / control / escalation) derived from the existing demo
  // projection. Generic action glyphs (settings cog, kebab, edit) are
  // explicitly NOT used — the card is an object identity surface, not
  // an action menu.
  //
  // Priority order (highest first, slice to two):
  //   1. critical / broken / dangling
  //   2. warning / missing / incomplete / "without grant"
  //   3. blocked
  //   4. suspended
  //   5. override / closed-only / strict / control / policy
  //   6. escalation
  //   7. healthy / active (used sparingly — only when no other
  //      condition applies)
  //
  // All inputs come from already-mapped node data (label, kind, id).
  // No invented counts, scores, or runtime metrics — the symbols are
  // honest derivations of demo labels documented in the impl-4c
  // report.

  var _AUTHORITY_SYMBOL_KEYS = {
    critical:   'severityCritical',
    warning:    'authorityFailModePolicy',
    blocked:    'stateBlocked',
    suspended:  'stateSuspended',
    override:   'authorityFailModePolicy',
    control:    'authorityFailModePolicy',
    escalation: 'authorityEscalationTarget',
    active:     'lifecycleActive',
  };

  function _strategicSymbolsForNode(ele) {
    if (!ele) return [];
    var label = '', kind = '', id = '';
    if (typeof ele.data === 'function') {
      label = String(ele.data('label') || '');
      kind  = String(ele.data('kind')  || '');
      id    = String(ele.data('id')    || '');
    } else {
      var d = ele.data || ele;
      label = String(d.label || '');
      kind  = String(d.kind  || '');
      id    = String(d.id    || '');
    }
    var hay = (label + ' ' + id).toLowerCase();
    var pal = _resolvePalette();
    var out = [];

    // Priority 1 — critical / broken / dangling.
    if (/\bdangling\b/.test(hay) || /\bbroken\b/.test(hay)) {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.critical, label: 'critical', stroke: pal.badgeBad });
    }
    // Priority 2 — warning / missing / incomplete / without grant.
    if (/\bmissing\b/.test(hay) || /without\s+grant/.test(hay) || /\bincomplete\b/.test(hay)) {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.warning, label: 'warning', stroke: pal.badgeWarn });
    }
    // Priority 3 — blocked.
    if (/\bblocked\b/.test(hay)) {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.blocked, label: 'blocked', stroke: pal.badgeBad });
    }
    // Priority 4 — suspended.
    if (/\bsuspended\b/.test(hay)) {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.suspended, label: 'suspended', stroke: pal.badgeWarn });
    }
    // Priority 5 — override / closed-only / strict / control / policy.
    if (/\boverride\b/.test(hay) || /closed[-\s]only/.test(hay) || /\bstrict\b/.test(hay)) {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.override, label: 'override', stroke: pal.slate400 });
    } else if (kind === 'fail_mode_policy') {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.control, label: 'control', stroke: pal.slate400 });
    }
    // Priority 6 — escalation (kind-driven).
    if (kind === 'escalation_target') {
      out.push({ key: _AUTHORITY_SYMBOL_KEYS.escalation, label: 'escalation', stroke: pal.slate400 });
    }

    // Cap at two highest-priority symbols.
    return out.slice(0, 2);
  }

  // ── D33a-spike-2g-impl-4 — Connector hover label vocabulary ──────────
  //
  // Maps Authority edge kinds to the concise display text used on
  // hover. The map is the source of truth for the seven Authority
  // edge kinds; unknown kinds fall back to the raw kind with
  // underscores rendered as spaces (matches the existing default).
  // Labels are intentionally short so they fit inside the per-edge
  // text-background chip without wrapping.
  var _AUTHORITY_EDGE_HOVER_LABELS = {
    business_service_has_surface:           'has surface',
    surface_uses_profile:                   'uses profile',
    profile_has_grant:                      'has grant',
    grant_authorises_agent:                 'authorises',
    surface_has_fail_mode_policy:           'fail-mode override',
    business_service_has_fail_mode_policy:  'default fail-mode',
    profile_escalates_to:                   'escalates to',
  };

  function _displayEdgeLabel(ele) {
    if (!ele) return '';
    var kind = '';
    if (typeof ele.data === 'function') {
      kind = String(ele.data('kind') || '');
    } else {
      var d = ele.data || ele;
      kind = String(d.kind || '');
    }
    if (_AUTHORITY_EDGE_HOVER_LABELS[kind]) {
      return _AUTHORITY_EDGE_HOVER_LABELS[kind];
    }
    return kind.replace(/_/g, ' ');
  }

  // ── Pure mapping: MIDAS projection → Cytoscape elements ──────────────
  //
  // Keeps enough metadata on each element to drive the inspector and
  // future overlays. Pure — no DOM, no fetch.

  function mapProjectionToElements(projection) {
    var out = { nodes: [], edges: [] };
    if (!projection || typeof projection !== 'object') return out;
    var nodes = Array.isArray(projection.nodes) ? projection.nodes : [];
    var edges = Array.isArray(projection.edges) ? projection.edges : [];
    var rootRef = projection.root || null;

    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (!n || !n.kind || !n.id) continue;
      var id = n.kind + ':' + n.id;
      var labelText = String(n.label || n.id || '');
      out.nodes.push({
        group: 'nodes',
        data: {
          id:    id,
          kind:  n.kind,
          label: labelText,
          name:  labelText,
          isRoot: !!(rootRef && rootRef.kind === n.kind && rootRef.id === n.id),
          raw:   n,
        },
      });
    }

    for (var j = 0; j < edges.length; j++) {
      var e = edges[j];
      if (!e || !e.kind || !e.src || !e.dst) continue;
      var srcId = e.src.kind + ':' + e.src.id;
      var dstId = e.dst.kind + ':' + e.dst.id;
      var eid   = e.kind + '|' + srcId + '→' + dstId;
      out.edges.push({
        group: 'edges',
        data: {
          id:     eid,
          source: srcId,
          target: dstId,
          kind:   e.kind,
          label:  String(e.kind || '').replace(/_/g, ' '),
          isSidecar: (e.kind === 'surface_has_fail_mode_policy'
                   || e.kind === 'business_service_has_fail_mode_policy'
                   || e.kind === 'profile_escalates_to'),
        },
      });
    }
    return out;
  }

  // ── Deterministic layout: lane × level preset positions ──────────────
  //
  // PoC layout mirrors the Authority Graph spec §5: one lane per
  // decision_surface; spine nodes (BS / surface / profile / grant /
  // agent) stack vertically. fail_mode_policy nodes are placed in a
  // governance side band so they're visible but don't dominate.

  var LAYOUT = {
    laneStride:    260,   // x spacing between lanes
    levelStride:   140,   // y spacing between spine levels
    rootY:         60,
    levelY:        { decision_surface: 200, authority_profile: 340,
                     authority_grant:  480, agent: 620 },
    sidecarOffset: 230,   // x offset from owner for FMP nodes
    sidecarY:      560,   // FMP band y
    canvasLeftPad: 80,
  };

  // D33a-spike-2g-impl-4d — Theme-aware effective layout. The
  // historical LAYOUT block hard-codes a 260 px lane stride and a
  // 140 px level stride that pre-dated the wider authority-thin-card
  // footprints. To keep the non-overlap policy honest (cards must
  // never touch) the active theme's `layoutNodeGapX` / `nodeW` and
  // `layoutNodeGapY` / `nodeH` tokens are honoured when present —
  // the effective lane stride becomes max(LAYOUT.laneStride,
  // nodeW + layoutNodeGapX). sidecarOffset is scaled proportionally
  // so the FMP band keeps its relative offset between lanes.
  //
  // Themes without these tokens fall through to LAYOUT unchanged,
  // preserving the historical geometry for classic / midas-card /
  // object-card / object-card-v2 / glass-card / holo-card / html-card
  // / object-tile-v3.
  function _effectiveLayout() {
    var L = {
      laneStride:    LAYOUT.laneStride,
      levelStride:   LAYOUT.levelStride,
      rootY:         LAYOUT.rootY,
      levelY:        LAYOUT.levelY,
      sidecarOffset: LAYOUT.sidecarOffset,
      sidecarY:      LAYOUT.sidecarY,
      canvasLeftPad: LAYOUT.canvasLeftPad,
    };
    var theme = _themeTokens(_activeTheme);
    if (!theme) return L;
    if (typeof theme.layoutNodeGapX === 'number' && typeof theme.nodeW === 'number') {
      var requiredLane = theme.nodeW + theme.layoutNodeGapX;
      if (requiredLane > L.laneStride) {
        // Scale sidecarOffset to preserve the FMP-vs-lane proportion.
        var scale = requiredLane / L.laneStride;
        L.sidecarOffset = Math.round(L.sidecarOffset * scale);
        L.laneStride = requiredLane;
      }
    }
    if (typeof theme.layoutNodeGapY === 'number' && typeof theme.nodeH === 'number') {
      var requiredLevel = theme.nodeH + theme.layoutNodeGapY;
      if (requiredLevel > L.levelStride) {
        L.levelStride = requiredLevel;
      }
      // D33a-spike-2g-impl-4e — derive the level-Y row map and the
      // sidecar band y from the new effective stride. The historical
      // `LAYOUT.levelY` and `LAYOUT.sidecarY` placed the sidecar
      // (fail-mode policy) row BETWEEN the grant and agent rows,
      // which made the dotted sidecar relationship lines visually
      // bleed into both neighbouring rows. The derived form below
      // stacks the spine rows on a uniform `stride` and places the
      // sidecar one full row BELOW the agent row so dotted
      // relationships cross only their own row band.
      var stride = L.levelStride;
      L.levelY = {
        decision_surface:  L.rootY + stride * 1,
        authority_profile: L.rootY + stride * 2,
        authority_grant:   L.rootY + stride * 3,
        agent:             L.rootY + stride * 4,
      };
      L.sidecarY = L.rootY + stride * 5;
    }
    return L;
  }

  function _computePresetPositions(projection, elements) {
    // Theme-aware effective layout — applies the impl-4d
    // non-overlap guard when the active theme declares layoutNodeGapX
    // / layoutNodeGapY. Other themes see the historical layout block.
    var _L = _effectiveLayout();
    // Build chain order from BS → Surface edges so lane assignment is
    // deterministic (mirrors the existing adapter's surface ordering).
    var edges = (projection && projection.edges) || [];
    var laneIdxBySurface = {};
    var laneCount = 0;
    for (var i = 0; i < edges.length; i++) {
      var e = edges[i];
      if (!e || e.kind !== 'business_service_has_surface' || !e.dst) continue;
      if (laneIdxBySurface[e.dst.id] != null) continue;
      laneIdxBySurface[e.dst.id] = laneCount++;
    }
    // Stragglers — any decision_surface node not reached via an edge.
    var nodes = (projection && projection.nodes) || [];
    for (var j = 0; j < nodes.length; j++) {
      var n = nodes[j];
      if (!n || n.kind !== 'decision_surface') continue;
      if (laneIdxBySurface[n.id] == null) laneIdxBySurface[n.id] = laneCount++;
    }

    // Resolve which surface owns each profile / grant / agent so the
    // downstream node inherits its lane. First-wins on shared nodes.
    var surfaceOfProfile = {}, profileOfGrant = {}, grantOfAgent = {};
    for (var k = 0; k < edges.length; k++) {
      var ee = edges[k];
      if (!ee || !ee.src || !ee.dst) continue;
      if (ee.kind === 'surface_uses_profile' && surfaceOfProfile[ee.dst.id] == null) {
        surfaceOfProfile[ee.dst.id] = ee.src.id;
      } else if (ee.kind === 'profile_has_grant' && profileOfGrant[ee.dst.id] == null) {
        profileOfGrant[ee.dst.id] = ee.src.id;
      } else if (ee.kind === 'grant_authorises_agent' && grantOfAgent[ee.dst.id] == null) {
        grantOfAgent[ee.dst.id] = ee.src.id;
      }
    }
    function laneFor(kind, id) {
      if (kind === 'decision_surface')  return laneIdxBySurface[id];
      if (kind === 'authority_profile') return laneIdxBySurface[surfaceOfProfile[id]];
      if (kind === 'authority_grant')   return laneIdxBySurface[surfaceOfProfile[profileOfGrant[id]]];
      if (kind === 'agent')             return laneIdxBySurface[surfaceOfProfile[profileOfGrant[grantOfAgent[id]]]];
      return undefined;
    }

    // Map fail_mode_policy nodes to their owner's lane for sidecar placement.
    var fmpOwnerLane = {};
    for (var m = 0; m < edges.length; m++) {
      var fe = edges[m];
      if (!fe || !fe.src || !fe.dst) continue;
      if (fe.kind === 'surface_has_fail_mode_policy') {
        if (fmpOwnerLane[fe.dst.id] == null) fmpOwnerLane[fe.dst.id] = laneIdxBySurface[fe.src.id];
      } else if (fe.kind === 'business_service_has_fail_mode_policy') {
        if (fmpOwnerLane[fe.dst.id] == null) fmpOwnerLane[fe.dst.id] = -1; // BS default → centre
      }
    }

    var positions = {};
    var rootX = _L.canvasLeftPad + ((Math.max(1, laneCount) - 1) * _L.laneStride) / 2;
    for (var p = 0; p < elements.nodes.length; p++) {
      var d = elements.nodes[p].data;
      if (d.kind === 'business_service') {
        positions[d.id] = { x: rootX, y: _L.rootY };
        continue;
      }
      if (d.kind === 'fail_mode_policy') {
        var ownerLane = fmpOwnerLane[d.raw.id];
        var fmpX = (ownerLane == null || ownerLane === -1)
          ? rootX + _L.sidecarOffset
          : _L.canvasLeftPad + ownerLane * _L.laneStride + _L.sidecarOffset;
        positions[d.id] = { x: fmpX, y: _L.sidecarY };
        continue;
      }
      var lane = laneFor(d.kind, d.raw.id);
      if (lane == null) {
        // Orphan — park below the agent row, distributed leftwards.
        positions[d.id] = { x: _L.canvasLeftPad + (p % 6) * _L.laneStride, y: _L.levelY.agent + 140 };
        continue;
      }
      var lvlY = _L.levelY[d.kind];
      if (lvlY == null) lvlY = _L.levelY.agent + 140;
      positions[d.id] = {
        x: _L.canvasLeftPad + lane * _L.laneStride,
        y: lvlY,
      };
    }
    return positions;
  }

  // ── Mount + Cytoscape lifecycle ──────────────────────────────────────

  var _cy = null;            // Cytoscape instance
  var _mountEl = null;       // <div> hosting the canvas
  var _legendEl = null;      // workbench legend overlay
  var _inspectorEl = null;   // PoC inspector panel
  var _lastProjection = null;
  // D33a-impl-1 — Collapsible chrome state. Legend defaults expanded
  // (reference info operators want visible); inspector defaults
  // collapsed (no node selected). Inspector auto-expands on tap.
  var _legendExpanded    = true;
  var _inspectorExpanded = false;

  // D33a-impl-1 — Compact / expanded widths used by _safeAreaPadding so
  // the Cytoscape fit padding never tucks the graph underneath the
  // legend / inspector chrome. Kept in sync with the CSS rules in
  // authority-cytoscape-poc.css.
  var LEGEND_W_COMPACT     = 140;
  var LEGEND_W_EXPANDED    = 260;
  var INSPECTOR_W_COMPACT  = 140;
  var INSPECTOR_W_EXPANDED = 320;

  // D33a-impl-1 — _safeAreaPadding reads MIDAS's `--gmap-overlay-inset-*`
  // tokens defined at [tokens.css:87-90] AND adds reserved-space for
  // the PoC's own legend / inspector chips. Cytoscape's `fit(eles,
  // padding)` accepts a single uniform value — we use the max of the
  // four side constraints so the graph never lands beneath a chrome
  // surface.
  //
  // D33a-impl-1a — Clamped against the mount's current dimensions.
  // Without the clamp, the computed value (e.g. left = 56 inset + 260
  // expanded-legend + 16 buffer = 332 px) can exceed the available
  // mount height on typical viewports (~500 px after workbench
  // chrome). Cytoscape's uniform fit would then subtract 2×332 from
  // each axis, leaving the graph with NEGATIVE vertical space and
  // rendering nothing visible (the D33a-impl-1 blank-canvas defect).
  // The clamp caps padding at min(mountW, mountH) / 5, so the graph
  // always gets at least 60 % of the smaller dimension.
  var FIT_PADDING_CAP_DIVISOR = 5;   // min(w, h) / 5
  var FIT_PADDING_FLOOR       = 24;  // never go below — keeps node bounds clear of borders
  var FIT_PADDING_HEADLESS    = 60;  // mount not measurable yet (init, test isolation)

  function _safeAreaPadding(dims) {
    var px = function (s) { return parseFloat(s) || 0; };
    var root, top = 0, right = 0, bottom = 0, left = 0;
    try {
      root = getComputedStyle(document.documentElement);
      top    = px(root.getPropertyValue('--gmap-overlay-inset-top'));
      right  = px(root.getPropertyValue('--gmap-overlay-inset-right'));
      bottom = px(root.getPropertyValue('--gmap-overlay-inset-bottom'));
      left   = px(root.getPropertyValue('--gmap-overlay-inset-left'));
    } catch (_) { /* fall through with zeros */ }
    var legendW    = _legendExpanded    ? LEGEND_W_EXPANDED    : LEGEND_W_COMPACT;
    var inspectorW = _inspectorExpanded ? INSPECTOR_W_EXPANDED : INSPECTOR_W_COMPACT;
    var buffer = 16;
    var computed = Math.max(
      top    + buffer,
      right  + inspectorW + buffer,
      bottom + buffer,
      left   + legendW    + buffer
    );

    // Resolve clamp dimensions. Prefer the explicit dims argument
    // (post-init paths read the live mount), fall back to the
    // module-state mount, and as a last resort use the headless cap.
    var w = 0, h = 0;
    if (dims && typeof dims.width === 'number' && typeof dims.height === 'number') {
      w = dims.width;
      h = dims.height;
    } else if (_mountEl) {
      w = _mountEl.clientWidth  || 0;
      h = _mountEl.clientHeight || 0;
    }
    if (w <= 0 || h <= 0) {
      // Mount not yet measurable — pick a conservative cap so the
      // initial fit doesn't overshoot. The post-init rAF guard
      // re-fits with real dimensions.
      return Math.max(FIT_PADDING_FLOOR, Math.min(computed, FIT_PADDING_HEADLESS));
    }
    var cap = Math.max(FIT_PADDING_FLOOR, Math.min(w, h) / FIT_PADDING_CAP_DIVISOR);
    return Math.max(FIT_PADDING_FLOOR, Math.min(computed, cap));
  }

  // D33a-impl-1 — _clearOverlays removes any PoC unavailable / loading
  // overlay from the mount. Called by _renderPayload before Cytoscape
  // init so a prior loading message cannot leak over the rendered
  // graph. Called by _renderUnavailable before appending so repeated
  // calls do not accumulate divs.
  function _clearOverlays(mount) {
    if (!mount) return;
    var prior = mount.querySelectorAll('.cytoscape-poc-unavailable');
    for (var i = 0; i < prior.length; i++) {
      if (prior[i].parentNode) prior[i].parentNode.removeChild(prior[i]);
    }
  }

  // D33a-impl-1 — Refit with the current safe-area padding. Called when
  // legend / inspector expand state changes so the visible graph never
  // ends up underneath a newly-expanded chrome surface.
  function _refitWithSafeArea() {
    if (!_cy) return;
    try {
      _cy.resize();
      _cy.fit(undefined, _safeAreaPadding());
    } catch (_) { /* swallow */ }
  }

  function _ensureMount() {
    if (_mountEl && _mountEl.isConnected) return _mountEl;
    var host = document.getElementById('gmap-canvas') || document.body;
    var parent = host.parentNode || document.body;

    _mountEl = document.createElement('div');
    _mountEl.id = 'gmap-cytoscape-mount';
    _mountEl.className = 'cytoscape-poc-mount';
    _mountEl.setAttribute('role', 'application');
    _mountEl.setAttribute('aria-label', 'Authority Graph (Cytoscape PoC)');
    parent.insertBefore(_mountEl, host);

    // Legend overlay — collapsible (D33a-impl-1). Header acts as a
    // toggle; data-expanded drives CSS show/hide of the body.
    _legendEl = document.createElement('aside');
    _legendEl.className = 'cytoscape-poc-legend';
    _legendEl.setAttribute('aria-label', 'Authority Graph workbench legend');
    _legendEl.setAttribute('data-expanded', _legendExpanded ? 'true' : 'false');
    _renderLegend(_legendEl);
    _mountEl.appendChild(_legendEl);

    // Inspector overlay — collapsible (D33a-impl-1). Auto-expands on
    // node tap; collapses on background tap.
    _inspectorEl = document.createElement('aside');
    _inspectorEl.className = 'cytoscape-poc-inspector';
    _inspectorEl.setAttribute('aria-label', 'Authority node inspector');
    _inspectorEl.setAttribute('data-expanded', _inspectorExpanded ? 'true' : 'false');
    _renderInspectorEmpty(_inspectorEl);
    _mountEl.appendChild(_inspectorEl);

    return _mountEl;
  }

  // D33a-impl-1 — _setLegendExpanded / _setInspectorExpanded update the
  // data-expanded attribute and trigger a fit recomputation with the
  // new safe-area padding. State changes are atomic — operators see a
  // single resize transition, not two staggered ones.
  function _setLegendExpanded(state) {
    _legendExpanded = !!state;
    if (_legendEl) _legendEl.setAttribute('data-expanded', _legendExpanded ? 'true' : 'false');
    _refitWithSafeArea();
  }
  function _setInspectorExpanded(state) {
    _inspectorExpanded = !!state;
    if (_inspectorEl) _inspectorEl.setAttribute('data-expanded', _inspectorExpanded ? 'true' : 'false');
    _refitWithSafeArea();
  }

  function _renderLegend(el) {
    var pal = _resolvePalette();
    el.innerHTML =
      // D33a-impl-1 — Header is a clickable toggle. The toggle button
      // surfaces collapse state to assistive tech via aria-expanded.
      // The PoC status badge replaces the prior #gmap-status hijack.
      '<button type="button" class="cytoscape-poc-toggle"' +
        ' data-poc-toggle="legend"' +
        ' aria-expanded="' + (_legendExpanded ? 'true' : 'false') + '"' +
        ' aria-controls="cytoscape-poc-legend-body">' +
        '<span class="cytoscape-poc-status-chip">PoC</span>' +
        '<span class="cytoscape-poc-toggle-label">Authority Graph</span>' +
        // D33a-spike-1 — Active theme name surfaced in the legend chip
        // header so the operator can tell which variant is rendering.
        '<span class="cytoscape-poc-theme-chip" data-poc-theme="' + _escHtml(_activeTheme) + '">' + _escHtml(_activeTheme) + '</span>' +
        '<span class="cytoscape-poc-toggle-glyph" aria-hidden="true">▾</span>' +
      '</button>' +
      '<div id="cytoscape-poc-legend-body" class="cytoscape-poc-legend-body">' +
        '<header class="cytoscape-poc-legend-title">Node kinds</header>' +
        '<dl class="cytoscape-poc-legend-kinds">' +
          _legendRow('Business Service',  pal.primary)    +
          _legendRow('Decision Surface',  pal.badgeInfo)  +
          _legendRow('Authority Profile', pal.badgeGood)  +
          _legendRow('Authority Grant',   pal.badgeWarn)  +
          _legendRow('Agent',             pal.onSurface)  +
          _legendRow('Fail-Mode Policy',  pal.slate400)   +
        '</dl>' +
        '<header class="cytoscape-poc-legend-title cytoscape-poc-legend-title-future">Future overlays</header>' +
        '<ul class="cytoscape-poc-legend-future">' +
          '<li><span class="cytoscape-poc-placeholder">●</span> Drift</li>' +
          '<li><span class="cytoscape-poc-placeholder">●</span> Resilience</li>' +
          '<li><span class="cytoscape-poc-placeholder">●</span> Diagnostics</li>' +
          '<li><span class="cytoscape-poc-placeholder">●</span> Runtime evidence</li>' +
        '</ul>' +
      '</div>';
    // Wire toggle click; pointerdown stops propagation so a stray
    // background-tap doesn't fire after a legend toggle.
    var btn = el.querySelector('[data-poc-toggle="legend"]');
    if (btn) {
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        _setLegendExpanded(!_legendExpanded);
        btn.setAttribute('aria-expanded', _legendExpanded ? 'true' : 'false');
      });
    }
  }
  function _legendRow(label, swatch) {
    return '<dt><span class="cytoscape-poc-swatch" style="background:' + swatch + '"></span></dt>' +
           '<dd>' + _escHtml(label) + '</dd>';
  }

  function _renderInspectorEmpty(el) {
    el.innerHTML =
      '<button type="button" class="cytoscape-poc-toggle"' +
        ' data-poc-toggle="inspector"' +
        ' aria-expanded="' + (_inspectorExpanded ? 'true' : 'false') + '"' +
        ' aria-controls="cytoscape-poc-inspector-body">' +
        '<span class="cytoscape-poc-toggle-label">Inspector</span>' +
        '<span class="cytoscape-poc-toggle-glyph" aria-hidden="true">▾</span>' +
      '</button>' +
      '<div id="cytoscape-poc-inspector-body" class="cytoscape-poc-inspector-body">' +
        '<p class="cytoscape-poc-inspector-empty">Click a node to inspect.</p>' +
      '</div>';
    _wireInspectorToggle(el);
  }
  function _renderInspector(node) {
    if (!_inspectorEl) return;
    var d = node.data();
    var raw = d.raw || {};
    var connected = node.connectedEdges().length;
    var fields = [];
    fields.push(['Kind',  _humanKind(d.kind)]);
    fields.push(['ID',    raw.id]);
    if (d.label && d.label !== raw.id) fields.push(['Label', d.label]);
    fields.push(['Connected edges', String(connected)]);
    if (d.isRoot) fields.push(['Root', 'business service']);
    var rows = fields.map(function (kv) {
      return '<dt>' + _escHtml(kv[0]) + '</dt><dd>' + _escHtml(String(kv[1])) + '</dd>';
    }).join('');
    _inspectorEl.innerHTML =
      '<button type="button" class="cytoscape-poc-toggle"' +
        ' data-poc-toggle="inspector"' +
        ' aria-expanded="' + (_inspectorExpanded ? 'true' : 'false') + '"' +
        ' aria-controls="cytoscape-poc-inspector-body">' +
        '<span class="cytoscape-poc-toggle-label">' + _escHtml(d.label || 'Inspector') + '</span>' +
        '<span class="cytoscape-poc-toggle-glyph" aria-hidden="true">▾</span>' +
      '</button>' +
      '<div id="cytoscape-poc-inspector-body" class="cytoscape-poc-inspector-body">' +
        '<dl class="cytoscape-poc-inspector-fields">' + rows + '</dl>' +
      '</div>';
    _wireInspectorToggle(_inspectorEl);
  }
  function _wireInspectorToggle(el) {
    var btn = el.querySelector('[data-poc-toggle="inspector"]');
    if (!btn) return;
    btn.addEventListener('click', function (e) {
      e.stopPropagation();
      _setInspectorExpanded(!_inspectorExpanded);
      btn.setAttribute('aria-expanded', _inspectorExpanded ? 'true' : 'false');
    });
  }

  function _humanKind(k) {
    return String(k || '').replace(/_/g, ' ').replace(/\b\w/g, function (c) { return c.toUpperCase(); });
  }

  // ── D33a-spike-2g-impl-5 — Inspector carrier DOM ─────────────────────
  //
  // The production right-side inspector (`authority-graph-inspector.js`)
  // selects a node by querying
  //
  //   document.getElementById('gmap-canvas')
  //     .querySelectorAll('.gmap-node[data-node-id=…]')
  //
  // and reads `dataset.nodeDetails` (a JSON blob) for the per-kind
  // formatter. The Cytoscape PoC renders to its own mount and never
  // paints `.gmap-node` cards, so the inspector cannot find a target
  // and bails out early at `if (!selectedNode) return;`.
  //
  // This carrier-DOM helper paints one hidden `.gmap-node` element per
  // mapped Cytoscape node under the production `#gmap-canvas`
  // container. The carrier exposes exactly the `dataset.*` contract
  // the production inspector expects — node id (kind:id refKey),
  // node name (full label), and node details JSON shaped to match
  // `_detailsFor()` in authority-graph-view.js:635-659.
  //
  // Carriers are:
  //   • presentation-free  (hidden + display:none + aria-hidden);
  //   • PoC-only           (marked with `data-cytoscape-poc-carrier`);
  //   • idempotent         (cleared before every render + on teardown);
  //   • read-only          (no event listeners, no Cytoscape coupling).
  //
  // Production rendering is unaffected because production paints its
  // own `.gmap-node` cards inside `#gmap-canvas`; the PoC carriers
  // live alongside them but are never visible (display: none).

  var _CARRIER_CLASS = 'cytoscape-poc-inspector-carrier';
  var _CARRIER_MARKER_ATTR = 'data-cytoscape-poc-carrier';

  // _detailsForCarrier composes the `data-node-details` JSON blob.
  // Shape mirrors `_detailsFor(node, adapter)` in the production view
  // (authority-graph-view.js:635-659): a flat object with `_kind`,
  // `_id`, `_label`, plus every key/value from the kind-specific
  // typed-data object on the projection node (e.g. `raw.decision_surface`,
  // `raw.authority_profile`). The PoC's mapped node carries the full
  // backend projection node under `nodeData.raw`, so we can reproduce
  // the production shape without an adapter round-trip.
  function _detailsForCarrier(nodeData, connectedCount) {
    var raw = (nodeData && nodeData.raw) || {};
    var kind = String(raw.kind || (nodeData && nodeData.kind) || '');
    var out = {
      _kind:  kind,
      _id:    String(raw.id != null ? raw.id : ''),
      _label: String(raw.label != null ? raw.label : (nodeData && nodeData.label) || ''),
    };
    // D33a-spike-2g-impl-5b — Attach the connected-edge count when
    // _renderInspectorCarriers passes one in. The MIDAS right drawer
    // inspector renders this as a "Connected edges" row, bringing
    // it to parity with the PoC inspector aside. The field stays
    // optional so production view (which doesn't compute it) keeps
    // working unchanged.
    if (typeof connectedCount === 'number' && isFinite(connectedCount) && connectedCount >= 0) {
      out._connected_edges = connectedCount;
    }
    // Flatten the kind-specific typed-data nested object exactly as
    // _detailsFor() does in production. Skip null / undefined; preserve
    // nested objects (constraints / capabilities / external_ref).
    if (kind && raw[kind] && typeof raw[kind] === 'object') {
      var typed = raw[kind];
      var keys = Object.keys(typed);
      for (var i = 0; i < keys.length; i++) {
        var k = keys[i];
        var v = typed[k];
        if (v == null) continue;
        out[k] = v;
      }
    }
    return out;
  }

  function _clearInspectorCarriers() {
    var host = document.getElementById('gmap-canvas');
    if (!host) return;
    var stale = host.querySelectorAll('[' + _CARRIER_MARKER_ATTR + ']');
    for (var i = 0; i < stale.length; i++) {
      var n = stale[i];
      if (n.parentNode) n.parentNode.removeChild(n);
    }
  }

  function _renderInspectorCarriers(elements) {
    var host = document.getElementById('gmap-canvas');
    if (!host) return;
    // Always clear stale carriers before re-render so successive
    // service switches / refreshes don't accumulate duplicates.
    _clearInspectorCarriers();
    var nodes = (elements && elements.nodes) || [];
    if (!nodes.length) return;
    // D33a-spike-2g-impl-5b — Pre-compute connected-edge counts so
    // the MIDAS right drawer inspector can show a "Connected edges"
    // row per node. The count is derived from the same `elements`
    // shape the renderer consumes — one pass over edges, O(N+E).
    var connectedByRef = {};
    var edgeList = (elements && elements.edges) || [];
    for (var ei = 0; ei < edgeList.length; ei++) {
      var ed = edgeList[ei] && edgeList[ei].data;
      if (!ed) continue;
      if (ed.source) connectedByRef[ed.source] = (connectedByRef[ed.source] || 0) + 1;
      if (ed.target) connectedByRef[ed.target] = (connectedByRef[ed.target] || 0) + 1;
    }
    // Use a single DocumentFragment so the host gets one DOM
    // insertion regardless of node count.
    var frag = document.createDocumentFragment();
    for (var i = 0; i < nodes.length; i++) {
      var entry = nodes[i];
      var d = entry && entry.data;
      if (!d || !d.id) continue;
      var carrier = document.createElement('div');
      carrier.className = 'gmap-node ' + _CARRIER_CLASS;
      carrier.setAttribute(_CARRIER_MARKER_ATTR, 'true');
      carrier.setAttribute('data-node-id', String(d.id));
      carrier.setAttribute('data-node-name', String(d.label || d.name || d.id));
      // JSON.stringify of the details object becomes the dataset
      // value; the production inspector reads it via
      // `JSON.parse(selectedNode.dataset.nodeDetails || '{}')`.
      try {
        var connected = connectedByRef[d.id] || 0;
        carrier.setAttribute('data-node-details', JSON.stringify(_detailsForCarrier(d, connected)));
      } catch (_) {
        // If anything inside `raw` is unserialisable, fall back to
        // the minimum production-required fields so the inspector
        // formatter still finds `_kind` / `_id` / `_label`.
        carrier.setAttribute('data-node-details', JSON.stringify({
          _kind:  String(d.kind || ''),
          _id:    String((d.raw && d.raw.id) || ''),
          _label: String(d.label || ''),
        }));
      }
      // Hide every which way: HTML5 hidden attribute, inline
      // display:none (defence against CSS resets), aria-hidden so
      // assistive tech ignores the carrier entirely.
      carrier.hidden = true;
      carrier.setAttribute('aria-hidden', 'true');
      carrier.style.display = 'none';
      frag.appendChild(carrier);
    }
    host.appendChild(frag);
  }
  function _escHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function _destroyCy() {
    // D33a-spike-2 — Tear down the html-card overlay before Cytoscape
    // so the overlay's event listeners can detach cleanly from the
    // still-live `_cy` instance. Repeated renders go through this
    // path, so no overlay DOM accumulates across service switches.
    _destroyHtmlCardOverlay();
    // D33a-spike-2g-impl-5 — Drop the inspector carriers before
    // Cytoscape teardown so successive renders cannot accumulate
    // stale `.gmap-node` carriers under #gmap-canvas. Idempotent.
    _clearInspectorCarriers();
    if (_cy) {
      try { _cy.destroy(); } catch (_) { /* swallow */ }
      _cy = null;
    }
  }

  // D33a-impl-1 — Full PoC teardown. Used by lensImpl.clear and the
  // public _destroy surface. Releases Cytoscape, removes every
  // PoC-owned DOM child (legend, inspector, unavailable overlays,
  // mount itself), and clears the body class so disabling the PoC
  // leaves zero residue. Idempotent: repeated calls are safe.
  function _uninstallPoc() {
    _destroyHtmlCardOverlay();
    // D33a-spike-2g-impl-5 — Belt-and-braces clear of any carrier
    // DOM the PoC dropped under #gmap-canvas. _destroyCy already
    // calls _clearInspectorCarriers; the duplicate call here keeps
    // _uninstallPoc idempotent if invoked outside the normal render
    // path (test teardown, manual diagnostics).
    _clearInspectorCarriers();
    _destroyCy();
    if (_mountEl && _mountEl.parentNode) {
      _clearOverlays(_mountEl);
      _mountEl.parentNode.removeChild(_mountEl);
    }
    _mountEl     = null;
    _legendEl    = null;
    _inspectorEl = null;
    if (document.body && document.body.classList) {
      document.body.classList.remove('cytoscape-poc-active');
    }
  }

  // ── D33a-spike-2 — html-card overlay lifecycle ────────────────────────
  //
  // The html-card theme renders a DOM card over each Cytoscape node.
  // Cytoscape continues to handle layout, edges, hover, drag, pan,
  // and zoom; the overlay is purely presentational. Container is
  // `pointer-events: none` so taps fall through to Cytoscape — there
  // are no interactive controls inside cards in this spike.
  //
  // Position-mapping uses node.renderedPosition() (vs cy.renderer().
  // projectIntoViewport()). Both APIs are available in Cytoscape
  // 3.30.2; renderedPosition returns viewport-relative coords
  // directly, no manual projection step required. It also fires
  // through the existing position-change events on every pan / zoom /
  // node-drag, so a single 'render' handler keeps cards in sync.
  //
  // The overlay container, card elements, and pan/zoom listener are
  // teared down by _destroyHtmlCardOverlay. Wired into both _destroyCy
  // (per-render teardown) and _uninstallPoc (full PoC teardown) so
  // service refresh and lens unmount both leave a clean DOM.

  var _htmlOverlayEl   = null;   // <div> containing cards
  var _htmlCardsByKey  = {};     // refKey -> <article> element
  var _htmlSyncRaf     = 0;      // pending rAF id; 0 = none
  var _htmlSyncBound   = null;   // bound listener for Cytoscape events

  function _installHtmlCardOverlay(cy, mount, elements, themeName) {
    if (!cy || !mount) return;
    if (themeName !== 'html-card') return;
    _destroyHtmlCardOverlay();

    _htmlOverlayEl = document.createElement('div');
    _htmlOverlayEl.className = 'cytoscape-poc-html-overlay';
    _htmlOverlayEl.setAttribute('role', 'presentation');
    mount.appendChild(_htmlOverlayEl);

    _htmlCardsByKey = {};
    var visibleNodes = (elements && elements.nodes) || [];
    for (var i = 0; i < visibleNodes.length; i++) {
      var entry = visibleNodes[i];
      if (!entry || !entry.data || !entry.data.id) continue;
      var card = _buildHtmlCard(entry.data);
      _htmlOverlayEl.appendChild(card);
      _htmlCardsByKey[entry.data.id] = card;
    }

    // Position cards immediately + on every pan/zoom/render/position event.
    _htmlSyncBound = function () {
      if (_htmlSyncRaf) return; // coalesce to one frame
      if (typeof window.requestAnimationFrame === 'function') {
        _htmlSyncRaf = window.requestAnimationFrame(function () {
          _htmlSyncRaf = 0;
          _updateHtmlCardOverlay(_cy);
        });
      } else {
        _updateHtmlCardOverlay(_cy);
      }
    };
    cy.on('render pan zoom position', _htmlSyncBound);
    _updateHtmlCardOverlay(cy);
  }

  function _buildHtmlCard(d) {
    var card = document.createElement('article');
    card.className = 'cytoscape-poc-html-card';
    card.setAttribute('data-node-id', d.id);
    card.setAttribute('data-kind', d.kind || '');
    if (d.isRoot) card.setAttribute('data-root', 'true');

    // Kind chip + title. No invented badges. Real data only.
    var kindEl = document.createElement('span');
    kindEl.className = 'cytoscape-poc-html-card-kind';
    kindEl.textContent = _nodeTypeLabel(d.kind || '');

    var titleEl = document.createElement('div');
    titleEl.className = 'cytoscape-poc-html-card-title';
    titleEl.textContent = String(d.label || d.id || '');

    card.appendChild(kindEl);
    card.appendChild(titleEl);

    // Status row — only emitted if the raw projection actually carries a
    // status field. No placeholder fakery. If the data isn't there, the
    // row is omitted entirely (an honest empty state).
    var raw = d.raw || {};
    var status =
      (raw.business_service  && raw.business_service.status) ||
      (raw.decision_surface  && raw.decision_surface.status) ||
      (raw.authority_profile && raw.authority_profile.status) ||
      (raw.authority_grant   && raw.authority_grant.status) ||
      (raw.agent             && raw.agent.operational_state) ||
      '';
    if (status) {
      var statusEl = document.createElement('div');
      statusEl.className = 'cytoscape-poc-html-card-status';
      statusEl.textContent = String(status);
      card.appendChild(statusEl);
    }
    return card;
  }

  function _updateHtmlCardOverlay(cy) {
    if (!cy || !_htmlOverlayEl) return;
    var keys = Object.keys(_htmlCardsByKey);
    for (var i = 0; i < keys.length; i++) {
      var id = keys[i];
      var card = _htmlCardsByKey[id];
      if (!card) continue;
      var n = cy.$id(id);
      if (!n || !n.length) {
        // Node no longer in graph (defensive). Hide the card.
        card.style.display = 'none';
        continue;
      }
      // renderedPosition returns viewport-relative coords (already
      // post-projection). Translate so the card centre aligns with
      // the Cytoscape node centre.
      var p = n.renderedPosition();
      var w = n.renderedWidth();
      var h = n.renderedHeight();
      var tx = Math.round(p.x - w / 2);
      var ty = Math.round(p.y - h / 2);
      card.style.transform = 'translate3d(' + tx + 'px, ' + ty + 'px, 0)';
      card.style.width  = Math.round(w) + 'px';
      card.style.height = Math.round(h) + 'px';
      card.style.display = '';
    }
  }

  function _destroyHtmlCardOverlay() {
    if (_htmlSyncRaf && typeof window.cancelAnimationFrame === 'function') {
      try { window.cancelAnimationFrame(_htmlSyncRaf); } catch (_) {}
    }
    _htmlSyncRaf = 0;
    if (_cy && _htmlSyncBound) {
      try { _cy.off('render pan zoom position', _htmlSyncBound); } catch (_) {}
    }
    _htmlSyncBound = null;
    if (_htmlOverlayEl && _htmlOverlayEl.parentNode) {
      _htmlOverlayEl.parentNode.removeChild(_htmlOverlayEl);
    }
    _htmlOverlayEl  = null;
    _htmlCardsByKey = {};
  }

  // ── Cytoscape style array ────────────────────────────────────────────

  // D33a-spike-1 — _themeTokens returns the per-theme node geometry +
  // typography baseline. Theme-specific properties (background-image,
  // overlay treatment, font weight) are applied directly inside
  // _buildStyleArray below. Keeping the descriptor flat avoids a
  // separate framework.
  function _themeTokens(themeName) {
    switch (themeName) {
      case 'midas-card':
        return {
          nodeW: 200, nodeH: 60, padding: '10px',
          fontSize: '12px', fontWeight: '500',
          borderWidth: 2, borderWidthFocused: 3, borderWidthRoot: 3,
          rootOverlayOpacity: 0.12,
          textMaxWidth: '160px',
        };
      case 'object-card':
        return {
          nodeW: 200, nodeH: 96, padding: '8px',
          fontSize: '11px', fontWeight: '500',
          borderWidth: 1.5, borderWidthFocused: 2.5, borderWidthRoot: 2.5,
          rootOverlayOpacity: 0.15,
          textMaxWidth: '170px',
          // Icons painted via background-image at the top of each node.
          iconHeight: '28px',
          iconWidth:  '28px',
          iconY:      '18%',
        };
      // D33a-spike-2 — Rich-theme descriptors. Each theme is wider /
      // taller than the spike-1 baseline so the readable label fits
      // without truncation and a two-line layout (title + TYPE) has
      // room to breathe.
      case 'object-card-v2':
        return {
          nodeW: 260, nodeH: 112, padding: '10px',
          fontSize: '13px', fontWeight: '600',
          borderWidth: 2, borderWidthFocused: 3, borderWidthRoot: 3,
          rootOverlayOpacity: 0.20,
          textMaxWidth: '210px',
          iconHeight: '28px',
          iconWidth:  '28px',
          iconY:      '18%',
          rootScale:  1.3,
        };
      case 'glass-card':
        return {
          nodeW: 240, nodeH: 96, padding: '10px',
          fontSize: '13px', fontWeight: '500',
          borderWidth: 1, borderWidthFocused: 2, borderWidthRoot: 2.5,
          rootOverlayOpacity: 0.18,
          textMaxWidth: '200px',
          rootScale:  1.3,
        };
      case 'holo-card':
        return {
          nodeW: 240, nodeH: 96, padding: '10px',
          fontSize: '13px', fontWeight: '600',
          borderWidth: 2, borderWidthFocused: 3, borderWidthRoot: 3,
          rootOverlayOpacity: 0.35,
          textMaxWidth: '200px',
          rootScale:  1.3,
        };
      case 'html-card':
        return {
          // Cytoscape node is intentionally small and faint — the HTML
          // overlay carries the visible card. Sizing kept consistent
          // with object-card-v2 so layout fits and the overlay anchors
          // align to the Cytoscape node centre.
          nodeW: 240, nodeH: 96, padding: '10px',
          fontSize: '12px', fontWeight: '500',
          borderWidth: 1, borderWidthFocused: 1.5, borderWidthRoot: 1.5,
          rootOverlayOpacity: 0,
          textMaxWidth: '200px',
          rootScale:  1.3,
        };
      // D33a-spike-2d — Native object tile. Compact icon-dominant body
      // with a separate label plate beneath, kind-shape variation,
      // gradient fill, ghost shadow, outline frame, taxi edges. This
      // descriptor declares the base values; the full theme rules are
      // appended at the end of _buildStyleArray inside the
      // `themeName === 'object-tile-v3'` branch.
      case 'object-tile-v3':
        return {
          nodeW: 108, nodeH: 78, padding: '4px',
          fontSize: '12px', fontWeight: '600',
          borderWidth: 1.5, borderWidthFocused: 2.5, borderWidthRoot: 2,
          rootOverlayOpacity: 0,
          textMaxWidth: '100px',
          // Root scaling left to the v3 branch — node-shape changes
          // mean width/height aren't a simple multiplication.
          rootScale:  1.3,
          // Compact icon size for the tile body.
          iconHeight: '40px',
          iconWidth:  '40px',
          iconY:      '50%',
        };
      // D33a-spike-2g-impl-4 — Authority thin card. Single uniform
      // footprint for every Authority node kind, left-aligned Lucide
      // icon, single readable native title. Root uses the same
      // width/height; only border weight / outline differs. Per-kind
      // shape variation is intentionally absent — mixing shapes hurt
      // readability in earlier spikes.
      //
      // D33a-spike-2g-impl-4a (superseded): added a two-line label
      // by composing title + kind subtitle.
      //
      // D33a-spike-2g-impl-4b: switched to the **fixed internal left
      // anchor** alignment model (text-halign 'right' + negative
      // text-margin-x). With halign='right' the label's bounding-box
      // left edge sits at `nodeOuterRight + titleMarginX` —
      // independent of label width, so short and long labels both
      // anchor to the same x. Calibration formula:
      //
      //   titleMarginX = -(outerWidth - desiredTitleStartX)
      //   text-max-width ≤ outerWidth - desiredTitleStartX
      //
      // See impl-4b precheck for the full derivation against
      // Cytoscape 3.30.2's recalculateNodeLabelProjection / Za.
      //
      // D33a-spike-2g-impl-4c: subtitle row removed. Card is a
      // single-line graph object (left icon + title + up to two
      // right-side strategic symbols). Card shrunk to 280×58 to
      // match the new single-line geometry.
      //
      // D33a-spike-2g-impl-4d: tuned for readability + non-overlap.
      // Title font bumped 13 → 14 px, card 280×58 → 290×60,
      // titleMarginX recalibrated, layoutNodeGapX / -Y added for the
      // non-overlap guard.
      //
      // D33a-spike-2g-impl-4e: title font further bumped 14 → 15 px
      // and weight 600 → 700 so the primary label reads cleanly at
      // default fit without zooming. Card grown 290×60 → 300×62 to
      // host the larger glyphs without crowding. titleMarginX
      // recalibrated from -240 → -248 against the new 316 px outer
      // width (316 − 248 = 68 px title start). titleMaxLen tightened
      // 24 → 18 chars so the 15 px / 700 title clamps before its
      // natural width exceeds the 180 px text-max-width. Readable
      // truncation is preferable to wrapped or compressed text.
      //
      // Vertical layout correction: layoutNodeGapY bumped 28 → 40
      // (>= the documented 36 px minimum) and `_effectiveLayout()`
      // now DERIVES the level-Y map (and sidecar row position) from
      // `rootY + stride * level` where stride = nodeH + gapY = 102.
      // The sidecar / FMP band sits at agent + stride (one full row
      // BELOW agent) so the dotted relationship lines no longer
      // visually bleed into adjacent rows.
      case 'authority-thin-card-v1':
        return {
          nodeW: 300, nodeH: 62, padding: '8px',
          fontSize: '15px', fontWeight: '700',
          lineHeight: 1.25,
          borderWidth: 1.5, borderWidthFocused: 2.5, borderWidthRoot: 2.5,
          rootOverlayOpacity: 0.18,
          textMaxWidth: '180px',
          iconHeight: '24px',
          iconWidth:  '24px',
          // Icon sits at the left edge, vertically centred.
          iconY:      '50%',
          iconX:      '12%',
          // Negative text-margin-x combined with text-halign 'right'
          // anchors the title's left edge at a fixed x position
          // independent of label width. See branch comment in
          // _buildStyleArray for the full geometry.
          titleMarginX: -248,
          // Title clamp for in-card display. Lower than impl-4d
          // because the larger 15 px / 700 glyphs occupy more
          // horizontal space per character.
          titleMaxLen: 18,
          // Strategic symbol slot positions (right-side zone). Two
          // slots; helper returns 0-2 symbols. 14px size matches the
          // restrained visual weight of the registry sub-icons.
          symbolW:    '14px',
          symbolH:    '14px',
          symbolY:    '50%',
          symbolX1:   '88%',
          symbolX2:   '94%',
          // Non-overlap layout guards (D33a-spike-2g-impl-4d +
          // -impl-4e). _effectiveLayout() honours these tokens and:
          //   • widens lane stride to nodeW + layoutNodeGapX so
          //     adjacent-lane cards never touch (horizontal);
          //   • DERIVES the level-Y map and sidecar row from
          //     rootY + stride*N where stride = nodeH + layoutNodeGapY
          //     so every spine row is ≥ nodeH + 36 apart and the
          //     sidecar band sits BELOW the agent row instead of
          //     between grant and agent.
          layoutNodeGapX: 30,
          layoutNodeGapY: 40,
        };
      case 'classic':
      default:
        return {
          nodeW: 180, nodeH: 56, padding: '10px',
          fontSize: '11px', fontWeight: 'normal',
          borderWidth: 1.5, borderWidthFocused: 3, borderWidthRoot: 3,
          rootOverlayOpacity: 0,
          textMaxWidth: '160px',
        };
    }
  }

  // D33a-spike-1 — Theme-aware style array. `themeName` selects which
  // descriptor / kind-overrides / icons to emit. All themes share the
  // same interaction-state selectors (.cy-dim / .cy-focused / .cy-
  // neighbor / .cy-on-root-path) so behaviour is identical across
  // themes; only appearance differs.
  function _buildStyleArray(themeName) {
    var theme = _themeTokens(themeName);
    var pal   = _resolvePalette();
    var kinds = _kindStyle(pal);

    // Base node + edge styles shared across themes.
    var base = [
      { selector: 'node', style: {
          'shape': 'round-rectangle',
          'background-color': pal.surfaceLow,
          'border-color': pal.outline,
          'border-width': theme.borderWidth,
          'color': pal.onSurface,
          'label': 'data(label)',
          'text-valign': (themeName === 'object-card') ? 'bottom' : 'center',
          'text-halign': 'center',
          'text-margin-y': (themeName === 'object-card') ? -10 : 0,
          'text-wrap': 'wrap',
          'text-max-width': theme.textMaxWidth,
          'font-size': theme.fontSize,
          'font-weight': theme.fontWeight,
          'font-family': "Inter, 'Segoe UI', system-ui, sans-serif",
          'padding': theme.padding,
          'width':  theme.nodeW,
          'height': theme.nodeH,
        }},
      { selector: 'node[?isRoot]', style: {
          'border-width': theme.borderWidthRoot,
          'border-color': pal.primary,
          'font-weight': 'bold',
        }},
      { selector: 'edge', style: {
          'curve-style': 'bezier',
          'control-point-step-size': 30,
          'width': 1.6,
          'line-color': pal.outline,
          'target-arrow-color': pal.outline,
          'target-arrow-shape': 'triangle',
          'arrow-scale': 0.9,
          'opacity': 0.7,
        }},
      { selector: 'edge[?isSidecar]', style: {
          'line-style': 'dashed',
          'line-dash-pattern': [6, 4],
          'line-color': pal.slate400,
          'target-arrow-color': pal.slate400,
        }},

      // Interaction states — identical across themes so behaviour is
      // unchanged whichever theme renders.
      { selector: '.cy-dim', style: {
          'opacity': 0.18,
          'text-opacity': 0.4,
        }},
      { selector: 'node.cy-focused', style: {
          'border-width': theme.borderWidthFocused,
          'border-color': pal.primary,
          'opacity': 1,
        }},
      { selector: 'node.cy-neighbor', style: {
          'border-width': 2,
          'border-color': pal.primary,
          'opacity': 1,
        }},
      { selector: 'edge.cy-focused', style: {
          'width': 2.4,
          'opacity': 1,
          'line-color': pal.primary,
          'target-arrow-color': pal.primary,
          'label': 'data(label)',
          'font-size': '10px',
          'color': pal.onSurfaceMut,
          'text-background-color': pal.surface,
          'text-background-opacity': 0.85,
          'text-background-padding': '2px',
        }},
      { selector: 'node.cy-on-root-path', style: {
          'border-color': pal.primary,
          'border-width': 2.5,
          'opacity': 1,
        }},
      { selector: 'edge.cy-on-root-path', style: {
          'line-color': pal.primary,
          'target-arrow-color': pal.primary,
          'width': 2.2,
          'opacity': 1,
        }},
    ];

    // Theme-specific overlays.
    if (themeName === 'midas-card' || themeName === 'object-card') {
      // Subtle primary glow around the root via a Cytoscape overlay.
      base.push({
        selector: 'node[?isRoot]',
        style: {
          'overlay-color':   pal.primary,
          'overlay-opacity': theme.rootOverlayOpacity,
          'overlay-padding': 4,
        },
      });
      // Selected nodes get a controlled ring via overlay-padding.
      base.push({
        selector: 'node:selected',
        style: {
          'overlay-color':   pal.primary,
          'overlay-opacity': 0.18,
          'overlay-padding': 3,
        },
      });
    }

    // Per-kind border + (for object-card / object-card-v2) type-icon
    // background. Later rules in the array take precedence in Cytoscape,
    // so theme-specific overrides below can layer onto this base.
    Object.keys(kinds).forEach(function (kind) {
      var ks = kinds[kind];
      var nodeStyle = {
        'background-color': pal.surfaceLow,
        'border-color': ks.stroke,
        'border-width': theme.borderWidth,
      };
      if (themeName === 'midas-card') {
        // Slightly heavier border + a touch of left-side accent via a
        // gradient overlay using the kind's stroke colour.
        nodeStyle['border-width'] = (kind === 'business_service') ? 3 : 2;
      } else if ((themeName === 'object-card' || themeName === 'object-card-v2') && _AUTHORITY_KIND_ICON_KEYS[kind]) {
        // Type tile: icon as background-image at the top of the card,
        // label text valign='bottom' so it sits beneath. background-fit
        // 'none' preserves the 28×28 icon size at all zooms.
        nodeStyle['background-image']     = _iconForKind(kind);
        nodeStyle['background-position-x'] = '50%';
        nodeStyle['background-position-y'] = theme.iconY;
        nodeStyle['background-width']      = theme.iconWidth;
        nodeStyle['background-height']     = theme.iconHeight;
        nodeStyle['background-fit']        = 'none';
        nodeStyle['background-clip']       = 'none';
        nodeStyle['background-image-opacity'] = 0.95;
      }
      base.push({
        selector: 'node[kind = "' + kind + '"]',
        style: nodeStyle,
      });
    });

    // ── D33a-spike-2 — Rich-theme overrides ─────────────────────────────
    //
    // Layered on top of the shared base so the existing classic /
    // midas-card / object-card themes remain byte-identical. Each rich
    // theme adds its visual signature (translucent fill / luminous
    // underlay / icon backplate) and shares the same interaction
    // selectors so hover / select / path-emphasis behave identically.

    // Rich themes get a richer two-line label "Title\nTYPE LABEL".
    // The label function references _displayLabel + _nodeTypeLabel so
    // long ids truncate without overflowing and the kind reads as an
    // implicit subtitle. html-card empties the label because the HTML
    // overlay carries the visible text.
    if (themeName === 'object-card-v2' ||
        themeName === 'glass-card'      ||
        themeName === 'holo-card') {
      base.push({
        selector: 'node',
        style: {
          'label': function (ele) {
            return _displayLabel(ele) + '\n' + _nodeTypeLabel(ele.data('kind')).toUpperCase();
          },
          'text-valign':   (themeName === 'object-card-v2') ? 'bottom' : 'center',
          'text-margin-y': (themeName === 'object-card-v2') ? -8 : 0,
          'line-height': 1.25,
        },
      });
    }
    if (themeName === 'html-card') {
      base.push({
        selector: 'node',
        style: {
          // HTML overlay carries the visible label; Cytoscape node is
          // a faint placeholder so failures of the overlay still leave
          // a recognisable shape on the canvas.
          'label': '',
          'background-opacity': 0.05,
          'border-opacity': 0.35,
          'border-style': 'dashed',
        },
      });
    }

    // glass-card: translucent fill + soft outline glow.
    if (themeName === 'glass-card') {
      base.push({
        selector: 'node',
        style: {
          'background-color':  '#222838',
          'background-opacity': 0.55,
          'underlay-color':    pal.outline,
          'underlay-opacity':  0.4,
          'underlay-padding':  1,
          'underlay-shape':    'round-rectangle',
        },
      });
    }

    // holo-card: near-black fill, kind-coloured luminous underlay glow.
    if (themeName === 'holo-card') {
      base.push({
        selector: 'node',
        style: {
          'background-color': '#0d1421',
        },
      });
      Object.keys(kinds).forEach(function (kind) {
        var ks = kinds[kind];
        base.push({
          selector: 'node[kind = "' + kind + '"]',
          style: {
            'underlay-color':   ks.stroke,
            'underlay-opacity': 0.35,
            'underlay-padding': 4,
            'underlay-shape':   'round-rectangle',
          },
        });
      });
      // Selected-path emphasis is even more luminous on holo-card.
      base.push({
        selector: 'node.cy-on-root-path',
        style: {
          'underlay-opacity': 0.6,
          'underlay-padding': 6,
        },
      });
      base.push({
        selector: 'edge.cy-on-root-path',
        style: { 'width': 3, 'opacity': 1 },
      });
    }

    // object-card-v2 / glass-card / holo-card: dashed border for
    // governance-overlay node kinds so they read as cross-cutting.
    if (themeName === 'object-card-v2' ||
        themeName === 'glass-card'      ||
        themeName === 'holo-card') {
      base.push({
        selector: 'node[kind = "fail_mode_policy"]',
        style: { 'border-style': 'dashed' },
      });
      base.push({
        selector: 'node[kind = "escalation_target"]',
        style: { 'border-style': 'dashed' },
      });
    }

    // Root scaling for rich themes — visually dominant but not oversized.
    if (theme.rootScale && theme.rootScale > 1.0) {
      base.push({
        selector: 'node[?isRoot]',
        style: {
          'width':  Math.round(theme.nodeW * theme.rootScale),
          'height': Math.round(theme.nodeH * theme.rootScale),
        },
      });
    }

    // ── D33a-spike-2d — object-tile-v3 ──────────────────────────────────
    //
    // Compact icon-dominant tile with separate label plate beneath,
    // gradient fill, ghost shadow, border + outline frame, kind-
    // dependent shape, taxi edge routing, transitions. Exercises every
    // unused-but-available Cytoscape capability identified in the
    // D33a-spike-2c styling audit.
    //
    // Rules pushed last so they override earlier base + per-kind +
    // rich-theme entries for this theme only.
    if (themeName === 'object-tile-v3') {
      // 1. Base node body — compact tile with gradient fill, ghost
      //    shadow, outline frame, label plate via text-background.
      //    `text-margin-y` positions the label below the tile so it
      //    reads as a separate chip rather than centred caption.
      base.push({
        selector: 'node',
        style: {
          // Compact tile geometry. Sized for icon dominance.
          'width':  theme.nodeW,
          'height': theme.nodeH,
          'padding': theme.padding,
          'shape': 'round-rectangle',
          // Gradient fill — replaces flat background-color.
          'background-fill':                'linear-gradient',
          'background-gradient-stop-colors':    '#2a3142 #1a1f2e',
          'background-gradient-stop-positions': '0% 100%',
          'background-gradient-direction':      'to-bottom',
          // Border + outline framing (outline is a SECOND line at
          // outline-offset px outside the border).
          'border-color':    pal.outline,
          'border-width':    theme.borderWidth,
          'border-style':    'solid',
          'outline-color':   pal.outline,
          'outline-width':   1,
          'outline-style':   'solid',
          'outline-opacity': 0.35,
          'outline-offset':  3,
          // Ghost shadow — Cytoscape's drop-shadow approximation.
          'ghost':          'yes',
          'ghost-offset-x': 0,
          'ghost-offset-y': 3,
          'ghost-opacity':  0.28,
          // Label plate — sits BELOW the tile via text-margin-y.
          'label': function (ele) { return _displayLabel(ele, 22); },
          'text-valign':         'bottom',
          'text-halign':         'center',
          'text-margin-y':       12,
          'text-wrap':           'ellipsis',
          'text-max-width':      theme.textMaxWidth,
          'text-justification':  'center',
          'color':               pal.onSurface,
          'font-size':           theme.fontSize,
          'font-weight':         theme.fontWeight,
          'font-family':         "Inter, 'Segoe UI', system-ui, sans-serif",
          'min-zoomed-font-size': 8,
          // The plate itself.
          'text-background-color':         '#1a1f2e',
          'text-background-opacity':       0.92,
          'text-background-shape':         'round-rectangle',
          'text-background-corner-radius': 3,
          'text-background-padding':       '4px',
          'text-border-color':             pal.outline,
          'text-border-width':             1,
          'text-border-opacity':           0.55,
          'text-border-style':             'solid',
          // Transitions so hover / select / dim feel animated.
          'transition-property':         'border-width opacity outline-offset underlay-padding underlay-opacity',
          'transition-duration':         180,
          'transition-timing-function':  'ease-out',
        },
      });

      // 2. Per-kind colour + shape + icon. Three distinct shapes
      //    (round-rectangle, round-hexagon, round-tag) — kept
      //    restrained so the graph reads as governed-objects rather
      //    than visual zoo.
      base.push({
        selector: 'node[kind = "business_service"]',
        style: {
          'shape': 'round-rectangle',
          'border-color':                       pal.primary,
          'text-border-color':                  pal.primary,
          'background-gradient-stop-colors':    '#324367 #1a1f2e',
          'background-image':                   _iconForKind('business_service'),
          'background-width':                   theme.iconWidth,
          'background-height':                  theme.iconHeight,
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "decision_surface"]',
        style: {
          'shape': 'round-rectangle',
          'border-color':                       pal.badgeInfo,
          'text-border-color':                  pal.badgeInfo,
          'background-gradient-stop-colors':    '#27334a #1a1f2e',
          'background-image':                   _iconForKind('decision_surface'),
          'background-width':                   theme.iconWidth,
          'background-height':                  theme.iconHeight,
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "authority_profile"]',
        style: {
          // Round-hexagon distinguishes authority constructs from
          // spine flow rectangles.
          'shape': 'round-hexagon',
          'border-color':                       pal.badgeGood,
          'text-border-color':                  pal.badgeGood,
          'background-gradient-stop-colors':    '#1f3a2c #1a1f2e',
          'background-image':                   _iconForKind('authority_profile'),
          'background-width':                   theme.iconWidth,
          'background-height':                  theme.iconHeight,
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "authority_grant"]',
        style: {
          // Round-tag distinguishes the grant "permission slip" shape.
          'shape': 'round-tag',
          'border-color':                       pal.badgeWarn,
          'text-border-color':                  pal.badgeWarn,
          'background-gradient-stop-colors':    '#3b2d18 #1a1f2e',
          'background-image':                   _iconForKind('authority_grant'),
          'background-width':                   theme.iconWidth,
          'background-height':                  theme.iconHeight,
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "agent"]',
        style: {
          'shape': 'round-rectangle',
          'border-color':                       pal.slate300,
          'text-border-color':                  pal.slate300,
          'background-gradient-stop-colors':    '#2c3344 #1a1f2e',
          'background-image':                   _iconForKind('agent'),
          'background-width':                   theme.iconWidth,
          'background-height':                  theme.iconHeight,
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "fail_mode_policy"]',
        style: {
          // Governance control treatment: dashed border + double
          // outline + slate accent. Reads as cross-cutting control,
          // not a chain participant.
          'shape':                              'round-rectangle',
          'border-color':                       pal.slate400,
          'border-style':                       'dashed',
          'text-border-color':                  pal.slate400,
          'text-border-style':                  'dashed',
          'outline-style':                      'double',
          'outline-width':                      2,
          'outline-color':                      pal.slate400,
          'outline-opacity':                    0.5,
          'background-gradient-stop-colors':    '#2a3140 #1a1f2e',
          'background-image':                   _iconForKind('fail_mode_policy'),
          'background-width':                   '34px',
          'background-height':                  '34px',
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });
      base.push({
        selector: 'node[kind = "escalation_target"]',
        style: {
          // Escalation: round-hexagon (mirrors profile geometry) but
          // dashed border to read as governance route, not artefact.
          'shape':                              'round-hexagon',
          'border-color':                       pal.slate400,
          'border-style':                       'dashed',
          'text-border-color':                  pal.slate400,
          'text-border-style':                  'dashed',
          'background-gradient-stop-colors':    '#2c2d3f #1a1f2e',
          'background-image':                   _iconForKind('escalation_target'),
          'background-width':                   '34px',
          'background-height':                  '34px',
          'background-position-x':              '50%',
          'background-position-y':              theme.iconY,
          'background-fit':                     'none',
          'background-repeat':                  'no-repeat',
          'background-clip':                    'none',
          'background-image-opacity':           0.95,
        },
      });

      // 3. Root emphasis — overrides the generic rich-theme root
      //    rule above. 1.3× scale + brighter gradient + thicker
      //    outline + bold label plate border.
      base.push({
        selector: 'node[?isRoot]',
        style: {
          'width':                              Math.round(theme.nodeW * theme.rootScale),
          'height':                             Math.round(theme.nodeH * theme.rootScale),
          'border-width':                       theme.borderWidthRoot,
          'outline-width':                      2,
          'outline-opacity':                    0.7,
          'outline-offset':                     4,
          'font-weight':                        'bold',
          'text-border-width':                  2,
          'text-border-opacity':                0.8,
          'background-gradient-stop-colors':    '#3c5288 #1a2438',
          'z-index':                            5,
        },
      });

      // 4. Interaction states — overlay + underlay + transition come
      //    together for a focused-node "rises and pulses" effect.
      //    z-index brings focused / selected / on-root-path elements
      //    above the rest of the graph.
      base.push({
        selector: 'node.cy-focused',
        style: {
          'border-width':       theme.borderWidthFocused,
          'outline-offset':     5,
          'outline-opacity':    0.7,
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.30,
          'underlay-padding':   5,
          'underlay-shape':     'round-rectangle',
          'overlay-color':      pal.primary,
          'overlay-opacity':    0.05,
          'overlay-padding':    2,
          'z-index':            10,
          'z-index-compare':    'manual',
        },
      });
      base.push({
        selector: 'node.cy-neighbor',
        style: {
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.18,
          'underlay-padding':   3,
          'underlay-shape':     'round-rectangle',
          'z-index':            7,
        },
      });
      base.push({
        selector: 'node.cy-on-root-path',
        style: {
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.40,
          'underlay-padding':   7,
          'underlay-shape':     'round-rectangle',
          'z-index':            8,
        },
      });
      base.push({
        selector: 'node:selected',
        style: {
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.35,
          'underlay-padding':   6,
          'underlay-shape':     'round-rectangle',
          'z-index':            9,
        },
      });

      // 5. Edge routing — taxi style for clean orthogonal flow.
      //    Spine edges flow downward; sidecar edges flow horizontally
      //    with hollow arrows + dashed line to read as control links.
      base.push({
        selector: 'edge',
        style: {
          'curve-style':            'taxi',
          'taxi-direction':         'downward',
          'taxi-turn':              '50%',
          'taxi-turn-min-distance': 12,
          'taxi-radius':            6,
          'edge-distances':         'intersection',
          'width':                  1.4,
          'opacity':                0.7,
          'line-color':             pal.outline,
          'target-arrow-color':     pal.outline,
          'target-arrow-shape':     'triangle',
          'arrow-scale':            0.85,
          'transition-property':         'width opacity line-color',
          'transition-duration':         180,
          'transition-timing-function':  'ease-out',
        },
      });
      base.push({
        selector: 'edge[?isSidecar]',
        style: {
          // Control / governance edges: horizontal taxi, dashed,
          // hollow arrow.
          'curve-style':           'taxi',
          'taxi-direction':        'horizontal',
          'line-style':            'dashed',
          'line-dash-pattern':     [4, 3],
          'line-color':            pal.slate400,
          'target-arrow-color':    pal.slate400,
          'target-arrow-shape':    'triangle',
          'target-arrow-fill':     'hollow',
        },
      });
      base.push({
        selector: 'edge.cy-focused',
        style: {
          'width':                          2.4,
          'opacity':                        1,
          'line-color':                     pal.primary,
          'target-arrow-color':             pal.primary,
          'label':                          'data(label)',
          'font-size':                      '10px',
          'color':                          pal.onSurfaceMut,
          'text-background-color':          pal.surface,
          'text-background-opacity':        0.95,
          'text-background-shape':          'round-rectangle',
          'text-background-corner-radius':  2,
          'text-background-padding':        '3px',
          'text-border-color':              pal.outline,
          'text-border-width':              1,
          'text-border-opacity':            0.4,
        },
      });
      base.push({
        selector: 'edge.cy-on-root-path',
        style: {
          'width':              2.2,
          'opacity':            1,
          'line-color':         pal.primary,
          'target-arrow-color': pal.primary,
        },
      });
    }

    // ── D33a-spike-2g-impl-4 — Authority thin card v1 ────────────────────
    //
    // Native Cytoscape thin-card theme inspired by the Palantir object
    // card pattern. Uniform 300×64 footprint, left-aligned 24px Lucide
    // icon, single readable title (`_displayTitle` strips Showcase /
    // Demo / grant-demo prefixes and collapses "FailModePolicy" →
    // "Policy"). Per-kind accent through border colour + kind-stroked
    // icon — never a saturated fill. Hover changes border / underlay /
    // opacity / z-index only; node width and height are constant.
    // Connector hover labels are 12 px minimum with a round-rectangle
    // chip background, revealed only on `edge.cy-focused` /
    // `edge.cy-on-root-path` — no rest-state edge labels.
    //
    // Card elements NOT implemented here (documented as future
    // contracts in the impl-4 report):
    //   • subtitle line
    //   • status chips
    //   • right action glyphs
    //   • hover context panel
    //   • workbench breadcrumb
    if (themeName === 'authority-thin-card-v1') {
      // 1. Re-bind node-wide properties: label is the normalised
      //    single-line _displayTitle (impl-4c removed the two-line
      //    subtitle row — node type is communicated by icon + colour
      //    + legend; details belong in the right-side letterbox /
      //    inspector after node click).
      //
      //    Alignment model (impl-4b, retained): text-halign 'right'
      //    + negative text-margin-x produces a **fixed internal left
      //    anchor**. From Cytoscape 3.30.2's
      //    recalculateNodeLabelProjection + label-bounding-box
      //    switch (function Za), for nodes:
      //
      //      labelX(halign='right') = nodeCentreX + nodeWidth/2 + padding
      //                              = node outer-right edge
      //      bbox.left = labelX + text-margin-x   (constant for any label width)
      //      bbox.right = labelX + text-margin-x + labelWidth
      //
      //    titleMarginX = -228 against a 296 px outer width (280 + 2·8)
      //    places the title start at 68 px from the card outer-left,
      //    well to the right of the 24 px icon at iconX 12 %.
      base.push({
        selector: 'node',
        style: {
          'shape':                          'round-rectangle',
          'background-color':               pal.surfaceLow,
          'background-opacity':             1,
          'border-color':                   pal.outline,
          'border-width':                   theme.borderWidth,
          'border-opacity':                 0.6,
          'label':                          function (ele) { return _displayTitle(ele, theme.titleMaxLen); },
          'font-size':                      theme.fontSize,
          'font-weight':                    theme.fontWeight,
          'line-height':                    theme.lineHeight,
          'color':                          pal.onSurface,
          'text-valign':                    'center',
          'text-halign':                    'right',
          'text-justification':             'left',
          'text-margin-x':                  theme.titleMarginX,
          'text-margin-y':                  0,
          'text-wrap':                      'wrap',
          'text-max-width':                 theme.textMaxWidth,
          'transition-property':            'border-color border-width underlay-opacity opacity',
          'transition-duration':            180,
          'transition-timing-function':     'ease-out',
        },
      });

      // 2. Per-kind accent + multi-image background.
      //    Function-valued styles compose the background-image array
      //    per node: [ baseTypeIcon, symbol0?, symbol1? ]. Cytoscape
      //    3.30.2 supports array-form background-image natively
      //    (vendored bundle, type a.urls). The corresponding
      //    background-position-x / -y / -width / -height / -fit /
      //    -repeat / -clip / -opacity arrays are aligned positionally.
      //
      //    Per-kind selectors carry the kind border + provide the
      //    kind stroke colour to the function bodies via closure
      //    capture of `kinds[kind]`.
      Object.keys(kinds).forEach(function (kind) {
        var ks = kinds[kind];
        if (!_AUTHORITY_KIND_ICON_KEYS[kind]) {
          base.push({
            selector: 'node[kind = "' + kind + '"]',
            style: {
              'border-color':   ks.stroke,
              'border-opacity': 0.85,
            },
          });
          return;
        }
        var baseIconForKind = _iconForKind(kind, ks.stroke);
        base.push({
          selector: 'node[kind = "' + kind + '"]',
          style: {
            'border-color':                ks.stroke,
            'border-opacity':              0.85,
            'background-image': function (ele) {
              var icons = window.MIDASExplorerIcons;
              var urls = [baseIconForKind];
              if (icons && typeof icons.cytoscapeDataURI === 'function') {
                var syms = _strategicSymbolsForNode(ele);
                for (var i = 0; i < syms.length && i < 2; i++) {
                  urls.push(icons.cytoscapeDataURI(syms[i].key, { stroke: syms[i].stroke }));
                }
              }
              return urls;
            },
            'background-position-x': function (ele) {
              var xs = [theme.iconX];
              var slots = [theme.symbolX1, theme.symbolX2];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) xs.push(slots[i]);
              return xs;
            },
            'background-position-y': function (ele) {
              var ys = [theme.iconY];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) ys.push(theme.symbolY);
              return ys;
            },
            'background-width': function (ele) {
              var ws = [theme.iconWidth];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) ws.push(theme.symbolW);
              return ws;
            },
            'background-height': function (ele) {
              var hs = [theme.iconHeight];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) hs.push(theme.symbolH);
              return hs;
            },
            'background-fit': function (ele) {
              var fs = ['none'];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) fs.push('none');
              return fs;
            },
            'background-repeat': function (ele) {
              var rs = ['no-repeat'];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) rs.push('no-repeat');
              return rs;
            },
            'background-clip': function (ele) {
              var cs = ['none'];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) cs.push('none');
              return cs;
            },
            'background-image-opacity': function (ele) {
              var os = [0.95];
              var syms = _strategicSymbolsForNode(ele);
              for (var i = 0; i < syms.length && i < 2; i++) os.push(0.95);
              return os;
            },
          },
        });
      });

      // 3. Root emphasis — same width / height (uniform card
      //    footprint), stronger border + soft primary outline glow.
      //    Card size is constant; root is signalled through colour
      //    weight, not geometry scale.
      base.push({
        selector: 'node[?isRoot]',
        style: {
          'width':              theme.nodeW,
          'height':             theme.nodeH,
          'border-color':       pal.primary,
          'border-width':       theme.borderWidthRoot,
          'outline-color':      pal.primary,
          'outline-width':      2,
          'outline-opacity':    0.5,
          'outline-offset':     3,
          'overlay-color':      pal.primary,
          'overlay-opacity':    theme.rootOverlayOpacity,
          'overlay-padding':    3,
          'font-weight':        'bold',
          'z-index':            5,
        },
      });

      // 4. Interaction states — never resize the card. Hover changes
      //    border / underlay / opacity / z-index only. The hovered
      //    node and its neighbours pop above unrelated graph; the
      //    rest dims via the shared `.cy-dim` rule.
      base.push({
        selector: 'node.cy-focused',
        style: {
          'border-color':       pal.primary,
          'border-width':       theme.borderWidthFocused,
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.22,
          'underlay-padding':   5,
          'underlay-shape':     'round-rectangle',
          'opacity':            1,
          'z-index':            10,
        },
      });
      base.push({
        selector: 'node.cy-neighbor',
        style: {
          'border-color':       pal.primary,
          'border-width':       2,
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.12,
          'underlay-padding':   3,
          'underlay-shape':     'round-rectangle',
          'opacity':            1,
          'z-index':            7,
        },
      });
      base.push({
        selector: 'node.cy-on-root-path',
        style: {
          'border-color':       pal.primary,
          'border-width':       2.5,
          'underlay-color':     pal.primary,
          'underlay-opacity':   0.30,
          'underlay-padding':   5,
          'underlay-shape':     'round-rectangle',
          'opacity':            1,
          'z-index':            8,
        },
      });

      // 5. Edge default — NO label at rest. Connector labels are
      //    hover-only by design (Palantir-inspired: relationships
      //    are revealed on focus, not always on screen).
      base.push({
        selector: 'edge',
        style: {
          'label':                          '',
          'width':                          1.6,
          'opacity':                        0.7,
          'transition-property':            'width opacity line-color',
          'transition-duration':            180,
          'transition-timing-function':     'ease-out',
        },
      });

      // 6. Connector hover label — chip styling on hovered /
      //    on-root-path edges only. Font ≥ 12 px, dark surface chip
      //    with a soft outline so it reads against any background.
      base.push({
        selector: 'edge.cy-focused',
        style: {
          'width':                            2.8,
          'opacity':                          1,
          'line-color':                       pal.primary,
          'target-arrow-color':               pal.primary,
          'label':                            function (ele) { return _displayEdgeLabel(ele); },
          'font-size':                        '12px',
          'font-weight':                      '500',
          'color':                            pal.onSurface,
          'text-background-color':            pal.surface,
          'text-background-opacity':          0.95,
          'text-background-shape':            'round-rectangle',
          'text-background-corner-radius':    3,
          'text-background-padding':          '5px',
          'text-border-color':                pal.outline,
          'text-border-width':                1,
          'text-border-opacity':              0.5,
        },
      });
      base.push({
        selector: 'edge.cy-on-root-path',
        style: {
          'width':                            3.2,
          'opacity':                          1,
          'line-color':                       pal.primary,
          'target-arrow-color':               pal.primary,
          'label':                            function (ele) { return _displayEdgeLabel(ele); },
          'font-size':                        '12px',
          'color':                            pal.onSurface,
          'text-background-color':            pal.surface,
          'text-background-opacity':          0.95,
          'text-background-shape':            'round-rectangle',
          'text-background-padding':          '5px',
        },
      });
    }

    return base;
  }

  // ── Interactions ─────────────────────────────────────────────────────

  function _clearInteractionState() {
    if (!_cy) return;
    _cy.elements().removeClass('cy-dim cy-focused cy-neighbor cy-on-root-path');
  }

  function _focusNode(node) {
    if (!_cy || !node) return;
    _clearInteractionState();
    var neighborhood = node.closedNeighborhood();
    _cy.elements().not(neighborhood).addClass('cy-dim');
    node.addClass('cy-focused');
    neighborhood.edges().addClass('cy-focused');
    neighborhood.nodes().not(node).addClass('cy-neighbor');
  }

  function _focusEdge(edge) {
    if (!_cy || !edge) return;
    _clearInteractionState();
    var both = edge.connectedNodes();
    _cy.elements().not(edge.union(both)).addClass('cy-dim');
    edge.addClass('cy-focused');
    both.addClass('cy-focused');
  }

  function _emphasiseRootPath(node) {
    // Walk predecessors back to a business_service via spine edges.
    if (!_cy || !node) return;
    var visited = {};
    var pathEdges = _cy.collection();
    var pathNodes = _cy.collection().union(node);
    var current = node;
    var hops = 0;
    while (current && hops < 16) {
      if (current.data('kind') === 'business_service') break;
      var inbound = current.incomers('edge[kind != "surface_has_fail_mode_policy"][kind != "business_service_has_fail_mode_policy"][kind != "profile_escalates_to"]');
      if (!inbound.length) break;
      var nextEdge = inbound[0];
      var prev = nextEdge.source();
      if (visited[prev.id()]) break;
      visited[prev.id()] = true;
      pathEdges = pathEdges.union(nextEdge);
      pathNodes = pathNodes.union(prev);
      current = prev;
      hops++;
    }
    pathNodes.addClass('cy-on-root-path');
    pathEdges.addClass('cy-on-root-path');
  }

  function _wireInteractions() {
    if (!_cy) return;

    _cy.on('mouseover', 'node', function (evt) { _focusNode(evt.target); });
    _cy.on('mouseout',  'node', function (evt) {
      // Restore baseline only if the node is not the click-selected one.
      var selected = _cy.$('node:selected');
      if (selected.length === 0) {
        _clearInteractionState();
      } else {
        _focusNode(selected[0]);
      }
      void evt;
    });

    _cy.on('mouseover', 'edge', function (evt) { _focusEdge(evt.target); });
    _cy.on('mouseout',  'edge', function (evt) {
      var selected = _cy.$('node:selected');
      if (selected.length === 0) {
        _clearInteractionState();
      } else {
        _focusNode(selected[0]);
      }
      void evt;
    });

    _cy.on('tap', 'node', function (evt) {
      var node = evt.target;
      _cy.elements().unselect();
      node.select();
      _focusNode(node);
      _emphasiseRootPath(node);
      _renderInspector(node);
      // D33a-impl-1 — Auto-expand the inspector on node selection so
      // the operator sees the metadata they just asked for. Padding
      // refit happens inside _setInspectorExpanded.
      if (!_inspectorExpanded) _setInspectorExpanded(true);
      // D33a-spike-2g-impl-5-precheck — Validate the selection
      // routing into the MIDAS right-side inspector. The renderer
      // hooks `selectNode(id)` shim wraps the inline
      // `selectGovernanceMapNode(id)`, which lens-dispatches into
      // `authorityInspector.selectNode(nodeId)` when the active lens
      // is 'authority'. The PoC's mapped node id (kind+':'+id)
      // already matches production's `_refKey({kind,id})` format, so
      // the same identifier flows through unchanged.
      //
      // Note (impl-5 next tranche): `authorityInspector.selectNode`
      // currently bails early when it cannot find a
      // `.gmap-node[data-node-id=…]` element under `#gmap-canvas` —
      // the PoC does not paint that DOM. This call therefore sets
      // `state.selectedId` + `gmapSelectedId` correctly, but the
      // right-side inspector rail formatter does not yet run. The
      // PoC inspector remains the visible source of truth until the
      // next tranche either teaches `selectNode` to accept an
      // in-memory payload or has the PoC emit carrier DOM elements.
      try {
        var hooks = window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks;
        var nodeId = node.data('id');
        if (hooks && typeof hooks.selectNode === 'function' && nodeId) {
          hooks.selectNode(nodeId);
        }
      } catch (_) { /* swallow — never break tap behaviour */ }
    });

    _cy.on('tap', function (evt) {
      if (evt.target === _cy) {
        // Background tap
        _cy.elements().unselect();
        _clearInteractionState();
        _renderInspectorEmpty(_inspectorEl);
        // D33a-impl-1 — Auto-collapse the inspector when selection
        // clears so the canvas isn't permanently encroached.
        if (_inspectorExpanded) _setInspectorExpanded(false);
      }
    });
  }

  // ── Render lifecycle ─────────────────────────────────────────────────

  function _renderPayload(payload) {
    var mount = _ensureMount();
    var Cytoscape = window.cytoscape;
    if (typeof Cytoscape !== 'function') {
      _renderUnavailable(mount, 'Cytoscape library not loaded.');
      return;
    }
    if (!payload || payload.__status === 404) {
      _renderUnavailable(mount, 'No Authority Graph for this service yet.');
      return;
    }
    if (payload && payload.__status === 501) {
      _renderUnavailable(mount, 'Authority Graph is not configured on this server.');
      return;
    }
    if (payload && payload.__status) {
      _renderUnavailable(mount, 'Authority Graph fetch failed (HTTP ' + payload.__status + ').');
      return;
    }

    _lastProjection = payload;
    // D33a-spike-2g-impl-5-precheck — Cache the projection on the
    // shared namespace so production-side consumers (authority graph
    // inspector overlay builder, authority workbench) can read
    // diagnostics / surface_posture for the selected node while the
    // PoC is active. The production `authority-graph-view.js` sets
    // this cache from its own render path; the PoC's lens-render
    // override never ran that code, so the cache was empty under
    // ?cytoscape=1. Populating it here parallels the production
    // contract without modifying the production view.
    if (window.MIDASExplorerGraph) {
      window.MIDASExplorerGraph._lastAuthorityProjection = payload;
    }
    var elements = mapProjectionToElements(payload);
    var positions = _computePresetPositions(payload, elements);

    // D33a-impl-1a — Defensive empty-elements check. Initialising
    // Cytoscape with zero nodes leaves the canvas blank; show an
    // explicit overlay so the user understands the lens has no data
    // rather than seeing a black rectangle.
    if (!elements.nodes.length) {
      _renderUnavailable(mount, 'Authority Graph has no nodes for this service.');
      return;
    }

    _destroyCy();
    // D33a-impl-1 — Clear any prior loading/unavailable overlay before
    // Cytoscape init so a stale "Loading…" message cannot leak through
    // onto the rendered graph. Mutually-exclusive states: success
    // removes overlay; _renderUnavailable also clears before re-append.
    _clearOverlays(mount);
    // D33a-spike-2g-impl-5a — Render hidden inspector carrier DOM
    // under #gmap-canvas AFTER _destroyCy() so the same-render
    // lifecycle does not wipe the carriers we just inserted (impl-5
    // had this call BEFORE _destroyCy, which clears carriers as part
    // of its teardown — the carriers therefore never survived to the
    // tap-handler routing). Production inspector code is unchanged;
    // carriers simply satisfy its existing DOM contract.
    _renderInspectorCarriers(elements);

    // D33a-impl-1 — Safe-area-aware fit padding. Reads MIDAS's
    // --gmap-overlay-inset-* tokens (tokens.css:87-90) and accounts
    // for the PoC's own legend / inspector reserved widths. Replaces
    // the pre-tranche hard-coded 60.
    // D33a-impl-1a — Read against the live mount so the value is
    // clamped to the actual available area (prevents the negative-
    // space defect that caused the blank canvas after D33a-impl-1).
    var fitPadding = _safeAreaPadding({
      width:  mount.clientWidth,
      height: mount.clientHeight,
    });

    // D33a-spike-2 — Mark the mount with the active theme so CSS rules
    // can scope to it (especially the html-card overlay rules). The
    // attribute is set/cleared per render so service-switching doesn't
    // strand a stale theme attribute on the mount.
    if (mount.dataset) mount.dataset.cyTheme = _activeTheme;

    _cy = window.cytoscape({
      container: mount,
      elements: elements.nodes.concat(elements.edges),
      // D33a-spike-1 — Theme selected by ?cyTheme=. Unknown values
      // fall back to 'classic' (the PoC baseline).
      style: _buildStyleArray(_activeTheme),
      layout: {
        name: 'preset',
        positions: function (n) { return positions[n.id()] || { x: 0, y: 0 }; },
        fit: true,
        padding: fitPadding,
      },
      wheelSensitivity: 0.2,
      boxSelectionEnabled: false,
      autounselectify: false,
    });

    _wireInteractions();

    // D33a-spike-2 — When the html-card theme is active, install the
    // DOM overlay. The overlay is purely presentational; pointer
    // events fall through to Cytoscape so drag/hover/select work
    // unimpeded. _destroyHtmlCardOverlay is wired into _destroyCy and
    // _uninstallPoc so service refresh / lens unmount leave no DOM.
    if (_activeTheme === 'html-card') {
      _installHtmlCardOverlay(_cy, mount, elements, _activeTheme);
    }

    // Post-init resize/fit guard. Cytoscape captures container
    // dimensions at init time; if the parent grid cell is still
    // laying out when we initialised (common with display: none
    // sibling toggles + flex/grid containers), the canvas can come up
    // with zero render area. requestAnimationFrame defers a resize +
    // re-fit to the next paint when the layout is settled.
    //
    // D33a-impl-1a — Strengthened to a double rAF (next frame + one
    // after) plus a 120 ms safety tick. The first rAF handles the
    // common case (layout settles in one frame). The nested rAF
    // catches the case where a parent grid track sizes only after
    // an additional pass. The setTimeout fallback covers slow
    // browsers / heavy DOM where two frames isn't enough.
    function _settleFit() {
      if (!_cy) return;
      try {
        _cy.resize();
        _cy.fit(undefined, _safeAreaPadding());
      } catch (_) { /* swallow */ }
    }
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () {
        _settleFit();
        window.requestAnimationFrame(_settleFit);
      });
    }
    setTimeout(_settleFit, 120);

    // D33a-impl-1 — No longer hijack #gmap-status. The PoC indicator
    // moved into the legend chip; the production status pill stays
    // available for the existing Authority workbench.
  }

  function _renderUnavailable(mount, message) {
    _destroyCy();
    // Re-mount the legend + inspector elements that _destroyCy removed
    // if Cytoscape had been initialised previously.
    if (_legendEl && !_legendEl.isConnected) mount.appendChild(_legendEl);
    if (_inspectorEl && !_inspectorEl.isConnected) mount.appendChild(_inspectorEl);
    // Remove any prior unavailable overlay before appending — repeated
    // refreshes must not accumulate divs.
    _clearOverlays(mount);
    var overlay = document.createElement('div');
    overlay.className = 'cytoscape-poc-unavailable';
    overlay.textContent = message;
    mount.appendChild(overlay);
  }

  // ── Lens dispatch override ───────────────────────────────────────────
  //
  // The renderer's register() overwrites prior registrations. Because
  // index.html loads this PoC AFTER authority-graph-view.js, we win
  // dispatch for the 'authority' lens whenever ?cytoscape=1 is active.
  // Removing this script from index.html restores the original.

  var lensImpl = {
    render: function (payload, mount) {
      void mount;
      _renderPayload(payload);
    },
    // D33a-impl-1 — `clear` now does a full PoC teardown rather than
    // just dropping the Cytoscape instance. Removes overlays, the
    // mount itself, and the body class so production Authority view
    // can resume cleanly if the PoC is unmounted.
    clear: function (mount) {
      void mount;
      _uninstallPoc();
    },
  };

  function _registerWhenReady() {
    var rendered = window.MIDASExplorerGraph && window.MIDASExplorerGraph.renderer;
    if (rendered && typeof rendered.register === 'function') {
      rendered.register('authority', lensImpl);
      return true;
    }
    return false;
  }

  if (!_registerWhenReady()) {
    // Renderer not yet attached — defer until DOMContentLoaded.
    document.addEventListener('DOMContentLoaded', function () { _registerWhenReady(); });
  }

  // ── authorityView.refresh override (PoC hotfix) ──────────────────────
  //
  // Diagnostic finding (D32h Cytoscape PoC blank-canvas pass):
  //   `renderer.render('authority', …)` has zero invocation sites in
  //   the live codebase. Authority renders flow exclusively through
  //   ExplorerGraph.authorityView.refresh({rootId}), which inside its
  //   IIFE calls the closure-local renderAuthorityGraph(payload, …)
  //   — bypassing the renderer dispatch table entirely.
  //
  //   The lens-registration override above is therefore unreachable in
  //   the current code. It's retained as a defensive fallback (in case
  //   future tranches route through renderer.render), but the primary
  //   interception now patches authorityView.refresh on the namespace.
  //   The two known call sites both go through the namespace lookup
  //   (index.html L2062-2064 for the route entry; L3251-3252 for the
  //   service-list click handler), so a single function replacement
  //   intercepts both paths.

  function _pocRefresh(opts) {
    opts = opts || {};
    var rootId = opts.rootId || '';
    var depth  = (typeof opts.depth === 'number' && opts.depth > 0) ? opts.depth : 4;

    if (!rootId) {
      _renderUnavailable(_ensureMount(),
        'Select a business service to view the Authority Graph.');
      return Promise.resolve({ __status: 0, __empty: true });
    }

    var adapter = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityAdapter;
    if (!adapter || typeof adapter.fetch !== 'function') {
      _renderUnavailable(_ensureMount(), 'Authority adapter unavailable.');
      return Promise.reject(new Error('authority adapter unavailable'));
    }

    // Show a loading state immediately so the user never sees a blank
    // canvas while the projection is fetched.
    _renderUnavailable(_ensureMount(), 'Loading…');

    return adapter.fetch({ view: 'service', id: rootId, depth: depth })
      .then(function (payload) {
        _renderPayload(payload);
        return payload;
      })
      .catch(function (err) {
        _renderUnavailable(_ensureMount(),
          'Network error: ' + ((err && err.message) || 'fetch failed'));
        throw err;
      });
  }

  function _patchAuthorityViewRefresh() {
    var av = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityView;
    if (av && typeof av.refresh === 'function') {
      // Preserve a back-reference for diagnostics / tests + uninstall.
      av._pocOriginalRefresh = av._pocOriginalRefresh || av.refresh;
      av.refresh = _pocRefresh;
      return true;
    }
    return false;
  }

  if (!_patchAuthorityViewRefresh()) {
    // authorityView is attached at the end of its IIFE, which runs
    // before this module thanks to the index.html script order, so the
    // immediate patch should normally succeed. Defer just in case of a
    // future load-order change.
    document.addEventListener('DOMContentLoaded', function () {
      _patchAuthorityViewRefresh();
    });
  }

  // ── Public surface (for test pinning + manual diagnostics) ───────────

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};
  window.MIDASExplorerGraph.cytoscapePoc = {
    isActive:                 _isPocActive,
    mapProjectionToElements:  mapProjectionToElements,
    _lensImpl:                lensImpl,
    _destroy:                 _destroyCy,
    // D33a-impl-1 — full teardown for tests and manual diagnostics.
    _uninstall:               _uninstallPoc,
    _safeAreaPadding:         _safeAreaPadding,
    // D33a-spike-1 — theme exploration surface.
    themes:                   _THEMES.slice(),
    activeTheme:               _activeTheme,
    _buildStyleArray:          _buildStyleArray,
    // D33a-spike-2g-impl-3 — registry-backed icon surface. The legacy
    // _objectCardIcons (self-authored data: URI map) has been removed;
    // these two entries expose the new contract for diagnostics.
    _authorityKindIconKeys:    _AUTHORITY_KIND_ICON_KEYS,
    _iconForKind:              _iconForKind,
    // D33a-spike-2 — display helpers + HTML overlay lifecycle.
    _displayLabel:             _displayLabel,
    _nodeTypeLabel:            _nodeTypeLabel,
    // D33a-spike-2g-impl-4 — thin-card title normaliser and connector
    // hover label vocabulary. Exposed for in-browser diagnostics.
    _displayTitle:             _displayTitle,
    _displayEdgeLabel:         _displayEdgeLabel,
    _authorityEdgeHoverLabels: _AUTHORITY_EDGE_HOVER_LABELS,
    // D33a-spike-2g-impl-4a — two-line thin-card label composer +
    // controlled subtitle vocabulary. (Retained for diagnostics and
    // future workbench breadcrumb work; the impl-4c visible card
    // label uses _displayTitle directly, not _displayCardLabel.)
    _displayCardLabel:         _displayCardLabel,
    _nodeSubtitle:             _nodeSubtitle,
    _nodeSubtitles:            _NODE_SUBTITLES,
    // D33a-spike-2g-impl-4c — right-side strategic symbol model.
    _strategicSymbolsForNode:  _strategicSymbolsForNode,
    _authoritySymbolKeys:      _AUTHORITY_SYMBOL_KEYS,
    // D33a-spike-2g-impl-5 — production-inspector carrier DOM.
    _renderInspectorCarriers:  _renderInspectorCarriers,
    _clearInspectorCarriers:   _clearInspectorCarriers,
    _detailsForCarrier:        _detailsForCarrier,
    _installHtmlCardOverlay:   _installHtmlCardOverlay,
    _updateHtmlCardOverlay:    _updateHtmlCardOverlay,
    _destroyHtmlCardOverlay:   _destroyHtmlCardOverlay,
  };
})();
