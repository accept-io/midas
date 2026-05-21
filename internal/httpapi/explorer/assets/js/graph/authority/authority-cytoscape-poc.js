// /explorer/assets/js/graph/authority/authority-cytoscape-poc.js
//
// Authority Graph — Cytoscape HTML-card renderer on GraphViewport.
//
// As of D37b this is the PRODUCTION Authority renderer. It registers
// with the GraphViewport host under the renderer id `'authority'`
// and is activated by normal Authority lens entry (no URL flag
// required). The pre-D37b `?cytoscape=1` gate has been retired.
//
// Internal-naming debt: the file is still
// `authority-cytoscape-poc.js`, the mount element class is still
// `.cytoscape-poc-mount`, and several internal symbols still carry
// `poc` in their name. Renaming would be a broad cross-file sweep
// affecting hundreds of test assertions; D37b leaves those names
// in place as documented internal naming debt and confines the
// strategic rename to the renderer-id literal exposed by
// GraphViewport (`'authority'`) and the user-facing surface (no
// "PoC" wording in aria-label / status copy).
//
// Mechanism:
//   • The module's IIFE registers `_authorityRendererFactory` with
//     GraphViewport under the id `'authority'` at module init.
//   • It also overrides the `'authority'` entry in the lens-agnostic
//     renderer dispatch table AND patches
//     `ExplorerGraph.authorityView.refresh` so the normal Authority
//     activation flow routes Authority projection fetches through
//     `_pocRefresh` and renders them via Cytoscape inside the
//     GraphViewport renderer slot.
//   • The pre-D37b "production native" path (renderAuthorityGraph in
//     authority-graph-view.js painting into `#gmap-canvas`) still
//     exists as legacy code, but is no longer the default user-facing
//     Authority path. A back-reference to the original refresh is
//     stored on the namespace (`_pocOriginalRefresh`) for diagnostics.
//
// Capabilities:
//   • Live Authority Graph data driving a Cytoscape canvas.
//   • Deterministic vertical-lane layout (preset positions).
//   • Hover behaviours: highlight focused node + its incident edges +
//     directly-connected neighbours; dim everything else.
//   • Edge hover: surface the relationship by emphasising endpoints.
//   • Click: select node; routes a payload through to the production
//     drawer/inspector via `_renderInspectorCarriers`.
//   • Background tap: clear selection / restore baseline.
//   • Path emphasis: walk predecessors back to the Business Service
//     root for any clicked spine node.
//   • Node dragging with auto-routed edges (Cytoscape native).
//   • Cached `_lastAuthorityProjection` so the existing diagnostics,
//     surface-posture, and workbench panels render against the same
//     payload.
//
// Known gaps still tracked by D37a §14 (G3 / G8 / G9):
//   • Inspector integration uses the carrier-DOM bridge; some
//     interactions remain less polished than the legacy native path.
//   • Authority Workbench / diagnostics / posture panels are bridged
//     via the cached projection but a richer integration is deferred
//     to D37c.
//   • Layer-state filtering, drift/resilience/diagnostics/runtime
//     overlay rendering, and full visual-semantics convergence with
//     D32h-fix-2e tokens are deferred follow-ups.

(function () {
  'use strict';

  // ── Activation ───────────────────────────────────────────────────────
  //
  // D37b — Pre-D37b the module was gated on `?cytoscape=1`. That
  // gate is RETIRED because the Cytoscape HTML-card renderer is now
  // the production Authority renderer. `_isPocActive` is preserved
  // as a public surface symbol (it returns `true` unconditionally
  // now) for any test/diagnostic call sites that still query it; do
  // not introduce new uses.

  function _isPocActive() {
    // D37b — always-on. Pre-D37b: `sp.get('cytoscape') === '1'`.
    return true;
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
  // D37f — Default theme promoted from `'classic'` to `'html-card'`.
  // The `'html-card'` theme descriptor declares the small/faint
  // 240×96 cy node footprint that matches the HTML card footprint —
  // a necessary condition for `cy.fit()` to land cards inside the
  // viewport when the HTML-card overlay is the default Authority
  // visual. The theme system + `?cyTheme=…` query param remain for
  // engineering exploration; HTML cards still render under any
  // theme (the overlay install is no longer gated by theme as of
  // D37f).
  var DEFAULT_THEME  = 'html-card';

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

  // D35f-retire-transitional-renderer-debt — Body-class activation
  // RETIRED. Pre-D35f the IIFE added `body.cytoscape-poc-active`
  // unconditionally on `?cytoscape=1`, which keyed two load-bearing
  // CSS rules: `#gmap-canvas { display: none !important }` and the
  // `.cytoscape-poc-mount { ... }` geometry. D35f moves both rules
  // onto `.midas-graph-viewport[data-active-renderer="authority"]`
  // (the renderer id used to be `"authority-cytoscape"`; D37b
  // promoted it to the production id `"authority"`). The
  // GraphViewport host (graph-viewport.js) sets this attribute
  // when `viewport.activateById('authority')` succeeds. The body
  // class is no longer the source of truth.

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
  var _lastProjection = null;
  // D33x-list-mode    — `_inspectorEl` + `_inspectorExpanded` retired;
  //                     production right drawer (`#gmap-details`) is
  //                     fed by the carrier DOM under `#gmap-canvas`
  //                     (see `cytoscape-poc-inspector-carrier`) plus
  //                     the `_rendererHooks.selectNode(nodeId)`
  //                     tap-handler dispatch.
  // D33x-left-poc-panel — `_legendEl` + `_legendExpanded` +
  //                     `_renderLegend` + `_setLegendExpanded` +
  //                     `_legendRow` + the `LEGEND_W_*` width
  //                     reservations retired. The floating
  //                     "Authority Graph" / NODE KINDS / FUTURE
  //                     OVERLAYS panel was PoC-only chrome; the
  //                     Authority projection's posture / layer
  //                     legend is owned by the MIDAS Posture & Help
  //                     drawer tab. Removing it frees up the left
  //                     side of the cy mount for the graph.

  // D33x-list-mode — Cytoscape-backed view mode. `graph` is the
  // default (preset layout with edges + spine positions); `list`
  // arranges nodes into a columnar layout grouped by kind and hides
  // edges. The mode is toggled by the workbench Form / Records
  // button when the active lens is `authority` and the PoC is
  // active (`body.cytoscape-poc-active`). Re-rendering the graph
  // (service refresh) resets to `graph`.
  var _viewMode = 'graph';
  // Snapshot of node positions captured the first time list mode is
  // entered after a render. Restored when exiting list mode so the
  // graph layout returns exactly as the PoC computed it (rather
  // than re-running the spine layout function each toggle).
  var _savedGraphPositions = null;

  // D33a-impl-1 — _safeAreaPadding reads MIDAS's `--gmap-overlay-inset-*`
  // tokens defined at [tokens.css:87-90] AND adds reserved-space for
  // the PoC's own legend / inspector chips. Cytoscape's `fit(eles,
  // padding)` accepts a single uniform value — we use the max of the
  // four side constraints so the graph never lands beneath a chrome
  // surface.
  //
  // D37q-viewport-4-impl — Per-edge safe-area handling:
  //   • The `cytoscape({layout: {padding: …}})` API and `cy.fit(eles,
  //     padding)` accept a SCALAR padding only. The `Math.max` collapse
  //     here is therefore forced by Cytoscape's API at the initial
  //     mount path.
  //   • The user-visible final fit comes from `_settleFit()` (double
  //     rAF + 120 ms timer) which delegates to `_fitToAvailableCanvas`
  //     below. That helper consumes per-edge insets (L / R / T / B)
  //     and applies them via `cy.viewport({zoom, pan})` — the
  //     canonical per-edge consumption point for Authority.
  //   • A future tranche could attempt per-edge at the initial mount
  //     too by computing the fit transform directly and bypassing
  //     `cytoscape({layout: {fit:true, padding: …}})` — out of scope
  //     here per the D37q-viewport-4 safe-implementation directive.
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
    // D33x-list-mode — Inspector reservation removed along with the
    // floating PoC card.
    // D33x-left-poc-panel — Legend reservation removed along with
    // the floating left PoC panel. Only the `--gmap-overlay-inset-*`
    // tokens + a symmetric buffer drive the padding now.
    var buffer = 16;
    var computed = Math.max(
      top    + buffer,
      right  + buffer,
      bottom + buffer,
      left   + buffer
    );

    // D35d-port-authority-cytoscape-to-graphviewport — Compose with
    // the GraphViewport host's safe-area. The host computes per-edge
    // insets from the live chrome geometry (`.gmap-mode-rail`,
    // `.gmap-camera-cluster`, `.gmap-legend-overlay`), which is
    // strictly more accurate than the static CSS-token values above.
    // Use Math.max so the legacy floor (CSS tokens) and the host
    // ceiling (live chrome) compose into a single scalar that never
    // lets the graph land beneath any chrome surface, regardless of
    // which signal is more conservative.
    //
    // Defensive: if the host or ctx is absent, the legacy `computed`
    // value is used unchanged.
    if (_rendererCtx && typeof _rendererCtx.getSafeArea === 'function') {
      try {
        var sa = _rendererCtx.getSafeArea();
        if (sa) {
          var hostMax = Math.max(sa.top || 0, sa.right || 0, sa.bottom || 0, sa.left || 0);
          if (hostMax > computed) computed = hostMax;
        }
      } catch (_) { /* swallow — keep legacy computed */ }
    }

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

  // D33x-fit-zoom-root — Bottom-band reservation for the MIDAS camera
  // cluster (`.gmap-camera-cluster`). The cluster is `position:
  // absolute; bottom: 16; right: 16; height: ~40 px`. Reserve enough
  // vertical room so the graph never lands beneath it.
  var TOOLBAR_BOTTOM_RESERVED_PX = 40 + 24;
  // D33x-fit-zoom-root — Top buffer. Above the cy canvas live the
  // search/breadcrumb chrome (`.governance-map-toolbar`) and the
  // shell header — both OUTSIDE the cy container. The buffer is
  // purely cosmetic so the top-most edges of the graph aren't flush
  // with the canvas border.
  var FIT_TOP_BUFFER_PX = 16;
  var FIT_SIDE_BUFFER_PX = 16;
  // Production right drawer (`#gmap-details.gmap-right-rail`) width
  // when expanded. Cytoscape lives inside the workbench grid cell
  // which shrinks when the drawer opens, so the drawer-induced inset
  // is already absorbed by `cy.resize()` reading the new container
  // width. The constant below is reserved as a defensive ceiling
  // when the drawer chrome may overlap (focus mode, transitioning
  // state); it is NOT double-counted against the natural cy.width().
  var RIGHT_DRAWER_OVERLAP_BUFFER = 0;
  // Minimum visible region the fit will preserve before it stops
  // applying per-side insets. Below this the math degenerates and
  // we just centre the graph at zoom = min(cw/bb.w, ch/bb.h).
  var FIT_MIN_VISIBLE_PX = 96;
  // Sane zoom clamps for the fit + the centre-on-root affordance.
  // Cytoscape's defaults (1e-50 .. 1e50) are unhelpful; these
  // values keep labels readable without ever going so close that
  // a single node fills the canvas.
  var FIT_ZOOM_MAX = 2.0;
  var FIT_ZOOM_MIN = 0.05;
  // Centre-on-root: if current zoom is already at least this, just
  // pan (preserves the operator's current detail level); otherwise
  // bump to `CENTRE_READABLE_ZOOM`.
  var CENTRE_READABLE_ZOOM = 0.85;
  // Zoom-in / zoom-out step factor. Mirrors the legacy
  // GMAP_ZOOM.STEP feel without coupling to that constant (which
  // lives in a different module).
  var ZOOM_STEP_FACTOR = 1.2;

  // _fitToAvailableCanvas computes per-side overlay-aware insets
  // and applies them via `cy.viewport({zoom, pan})`. Cytoscape's
  // built-in `cy.fit(eles, padding)` takes only a SCALAR padding,
  // which forces the largest single-side overlay onto every side
  // (e.g. expanded legend = 260 + buffer = 276 px would reserve
  // 276 on top + 276 on bottom too, leaving the graph cramped).
  // This helper measures the insets independently per side so the
  // graph uses the full vertical height plus the centre horizontal
  // band between the legend and the inspector.
  //
  // Steps:
  //   1. Read the elements' bounding box.
  //   2. Compute per-side insets (legend = left only, inspector =
  //      right only, camera cluster = bottom only, small top
  //      buffer).
  //   3. Derive zoom = min(visibleWidth/bb.w, visibleHeight/bb.h)
  //      and clamp against the chosen min/max + Cytoscape's own
  //      min/max.
  //   4. Derive pan so the bb centre lands at the centre of the
  //      visible region.
  //   5. Apply zoom + pan atomically via `cy.viewport()`.
  function _fitToAvailableCanvas(cy, opts) {
    if (!cy) return;
    opts = opts || {};
    var eles;
    try {
      eles = opts.elements || cy.elements(':visible');
    } catch (_) {
      eles = cy.elements();
    }
    if (!eles || eles.length === 0) return;
    var bb;
    try { bb = eles.boundingBox(); } catch (_) { return; }
    if (!bb || bb.w <= 0 || bb.h <= 0) return;

    var cw = (typeof cy.width  === 'function') ? cy.width()  : 0;
    var ch = (typeof cy.height === 'function') ? cy.height() : 0;
    if (cw <= 0 || ch <= 0) return;

    // D33x-list-mode    — Inspector reservation removed; right drawer
    //                     naturally shrinks `cy.width()` when it
    //                     opens, so the right inset is just the
    //                     symmetric side buffer plus any defensive
    //                     overlap allowance.
    // D33x-left-poc-panel — Legend reservation removed; the floating
    //                     left "Authority Graph" PoC panel was
    //                     retired so the left inset is just the
    //                     symmetric side buffer.

    var L = FIT_SIDE_BUFFER_PX;
    var R = FIT_SIDE_BUFFER_PX + RIGHT_DRAWER_OVERLAP_BUFFER;
    var T = FIT_TOP_BUFFER_PX;
    var B = TOOLBAR_BOTTOM_RESERVED_PX;

    // Degenerate-viewport guard. If the insets would collapse the
    // visible region below `FIT_MIN_VISIBLE_PX`, scale the side
    // insets back proportionally so the graph stays visible. A
    // 480 px-wide mount in a narrow split pane still gets a usable
    // visible region.
    if (cw - L - R < FIT_MIN_VISIBLE_PX) {
      var hSlack = cw - FIT_MIN_VISIBLE_PX;
      var hWeight = L + R;
      if (hSlack > 0 && hWeight > 0) {
        L = Math.max(FIT_SIDE_BUFFER_PX, L * hSlack / hWeight);
        R = Math.max(FIT_SIDE_BUFFER_PX, R * hSlack / hWeight);
      } else {
        L = FIT_SIDE_BUFFER_PX;
        R = FIT_SIDE_BUFFER_PX;
      }
    }
    if (ch - T - B < FIT_MIN_VISIBLE_PX) {
      var vSlack = ch - FIT_MIN_VISIBLE_PX;
      var vWeight = T + B;
      if (vSlack > 0 && vWeight > 0) {
        T = Math.max(FIT_TOP_BUFFER_PX / 2, T * vSlack / vWeight);
        B = Math.max(FIT_TOP_BUFFER_PX / 2, B * vSlack / vWeight);
      } else {
        T = FIT_TOP_BUFFER_PX / 2;
        B = FIT_TOP_BUFFER_PX / 2;
      }
    }

    var vw = Math.max(FIT_MIN_VISIBLE_PX, cw - L - R);
    var vh = Math.max(FIT_MIN_VISIBLE_PX, ch - T - B);

    var z = Math.min(vw / bb.w, vh / bb.h);
    // Cytoscape's own min/max — respect them so we never push the
    // viewport into a regime Cytoscape will then clamp back.
    var cyMax = (typeof cy.maxZoom === 'function') ? cy.maxZoom() : Infinity;
    var cyMin = (typeof cy.minZoom === 'function') ? cy.minZoom() : 0;
    z = Math.min(z, isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);
    z = Math.max(z, cyMin || 0, FIT_ZOOM_MIN);

    // Render the bb centre at the centre of the visible region.
    var rcx = L + vw / 2;
    var rcy = T + vh / 2;
    var gx = bb.x1 + bb.w / 2;
    var gy = bb.y1 + bb.h / 2;

    try {
      cy.viewport({
        zoom: z,
        pan: { x: rcx - gx * z, y: rcy - gy * z },
      });
    } catch (_) { /* swallow — Cytoscape silently rejects invalid */ }
  }

  // _zoomBy multiplies the current zoom by `factor`, anchored at the
  // current viewport centre (NOT the cursor). Centre-anchoring keeps
  // the graph's apparent centre stable across +/- presses, matching
  // the legacy MIDAS toolbar feel.
  function _zoomBy(cy, factor) {
    if (!cy || !factor || !isFinite(factor)) return;
    var cw = (typeof cy.width  === 'function') ? cy.width()  : 0;
    var ch = (typeof cy.height === 'function') ? cy.height() : 0;
    if (cw <= 0 || ch <= 0) return;
    var current = (typeof cy.zoom === 'function') ? cy.zoom() : 1;
    var next = current * factor;
    var cyMax = (typeof cy.maxZoom === 'function') ? cy.maxZoom() : FIT_ZOOM_MAX;
    var cyMin = (typeof cy.minZoom === 'function') ? cy.minZoom() : FIT_ZOOM_MIN;
    next = Math.min(next, isFinite(cyMax) ? cyMax : FIT_ZOOM_MAX);
    next = Math.max(next, cyMin || FIT_ZOOM_MIN);
    try {
      cy.zoom({
        level: next,
        renderedPosition: { x: cw / 2, y: ch / 2 },
      });
    } catch (_) { /* swallow */ }
  }

  // _findRootNode locates the Authority projection's root in the
  // Cytoscape elements. Resolution order:
  //   1. A node whose data carries `isRoot === true` (set by
  //      mapProjectionToElements when projection.root matches).
  //   2. A node whose data.kind is `business_service` (Authority
  //      projection invariant: business_service is the layout root).
  //   3. null — caller logs a debug message and returns.
  function _findRootNode(cy) {
    if (!cy) return null;
    var roots;
    try {
      roots = cy.nodes().filter(function (n) {
        var v = n.data('isRoot');
        return v === true || v === 'true' || v === 1;
      });
    } catch (_) { roots = null; }
    if (roots && roots.length > 0) return roots[0];
    var bs;
    try {
      bs = cy.nodes().filter(function (n) {
        var d = n.data();
        if (!d) return false;
        if (d.kind === 'business_service') return true;
        if (d.raw && d.raw.kind === 'business_service') return true;
        return false;
      });
    } catch (_) { bs = null; }
    if (bs && bs.length > 0) return bs[0];
    return null;
  }

  // _centerOnRoot pans (and optionally zooms) so the projection's
  // root is centred and readable. Preserves the operator's current
  // zoom if it's already at least `CENTRE_READABLE_ZOOM`; otherwise
  // bumps zoom to `CENTRE_READABLE_ZOOM` so labels are legible.
  //
  // D37j — Centre-on-root exits any active authority-context view
  // first. The legacy semantic "return to the projection root" is
  // operationally only meaningful against the full graph.
  function _centerOnRoot(cy) {
    if (!cy) return;
    _exitAuthorityContext();
    var root = _findRootNode(cy);
    if (!root) {
      if (window.console && typeof window.console.debug === 'function') {
        try {
          window.console.debug('cytoscape-poc: centre-on-root — no root node found');
        } catch (_) { /* swallow */ }
      }
      return;
    }
    var current = (typeof cy.zoom === 'function') ? cy.zoom() : 1;
    var target = current;
    if (current < CENTRE_READABLE_ZOOM) {
      target = CENTRE_READABLE_ZOOM;
    }
    var cyMax = (typeof cy.maxZoom === 'function') ? cy.maxZoom() : FIT_ZOOM_MAX;
    if (isFinite(cyMax)) target = Math.min(target, cyMax);
    target = Math.min(target, FIT_ZOOM_MAX);
    target = Math.max(target, FIT_ZOOM_MIN);
    var cw = cy.width(), ch = cy.height();
    if (cw <= 0 || ch <= 0) return;
    var p;
    try { p = root.position(); } catch (_) { p = null; }
    if (!p) return;
    try {
      cy.viewport({
        zoom: target,
        pan: { x: cw / 2 - p.x * target, y: ch / 2 - p.y * target },
      });
    } catch (_) { /* swallow */ }
  }

  // ── D37h — Camera/navigation helpers for toolbar ─────────────────────
  //
  // Strict camera-only surface: every helper here operates on the
  // current Cytoscape viewport state. None of them mutate the graph
  // contents, filter the projection, hide elements, or re-query the
  // backend. HTML cards stay pointer-passive and are kept aligned by
  // the D37f two-tier overlay sync.

  // _zoomToNode focuses the camera on a single Cytoscape node using
  // the existing safe-area-aware `cy.fit(eles, padding)` model. It is
  // a thin wrapper over `_fitToAvailableCanvas` so the visible-area
  // calculation (inspector drawer, focus-mode chrome insets) is
  // reused.
  function _zoomToNode(cy, node) {
    if (!cy || !node) return;
    if (typeof node.length === 'number' && node.length === 0) return;
    try {
      _fitToAvailableCanvas(cy, { elements: node });
    } catch (_) { /* swallow */ }
  }

  // _zoomToSelected focuses the camera on `cy.elements(':selected')`.
  // When more than one node is selected the camera fits all of them
  // (the simpler choice — documented in the D37h tranche report);
  // when nothing is selected it is a no-op (the toolbar disables the
  // button in that state anyway).
  function _zoomToSelected(cy) {
    if (!cy || typeof cy.elements !== 'function') return;
    var selected;
    try { selected = cy.elements(':selected'); } catch (_) { return; }
    if (!selected || typeof selected.length !== 'number' || selected.length === 0) return;
    _zoomToNode(cy, selected);
  }

  // _resetView restores a sensible whole-graph Authority camera. It
  // intentionally does NOT use raw `cy.reset()` (which would set
  // pan=(0,0), zoom=1 and ignore MIDAS safe-area chrome). Instead it
  // delegates to the same `_fitToAvailableCanvas` helper that drives
  // the Fit button, so the result respects inspector / focus-mode /
  // camera-cluster insets.
  //
  // D37j — Reset exits any active authority-context view first so
  // the user lands on the full Business-Service-rooted graph rather
  // than a "fit-on-context" that looks identical to the unfit
  // context view.
  function _resetView(cy) {
    if (!cy) return;
    try {
      _exitAuthorityContext();
      if (typeof cy.elements === 'function') {
        try { cy.elements().unselect(); } catch (_) { /* swallow */ }
      }
      _clearInteractionState();
      _fitToAvailableCanvas(cy);
    } catch (_) { /* swallow */ }
  }

  // _getZoomPercent reads `cy.zoom()` and returns it as a whole
  // percentage. Returns null when no cy is mounted so the toolbar
  // can render a `--%` placeholder without making one up.
  function _getZoomPercent(cy) {
    if (!cy || typeof cy.zoom !== 'function') return null;
    var z;
    try { z = cy.zoom(); } catch (_) { return null; }
    if (typeof z !== 'number' || !isFinite(z) || z <= 0) return null;
    return Math.round(z * 100);
  }

  // External viewport- and selection-change subscribers. The
  // registries survive cy teardown so re-mounts re-attach the same
  // subscribers to the fresh cy instance — subscribers do not need
  // to know about the cy lifecycle.
  var _viewportChangeHandlers  = [];
  var _selectionChangeHandlers = [];

  function _attachExternalHandlersToCy(cy) {
    if (!cy || typeof cy.on !== 'function') return;
    for (var i = 0; i < _viewportChangeHandlers.length; i++) {
      try { cy.on('zoom pan resize', _viewportChangeHandlers[i]); } catch (_) {}
    }
    for (var j = 0; j < _selectionChangeHandlers.length; j++) {
      try { cy.on('select unselect', _selectionChangeHandlers[j]); } catch (_) {}
    }
  }

  function _onViewportChanged(handler) {
    if (typeof handler !== 'function') return function () {};
    _viewportChangeHandlers.push(handler);
    if (_cy) { try { _cy.on('zoom pan resize', handler); } catch (_) {} }
    return function () {
      var idx = _viewportChangeHandlers.indexOf(handler);
      if (idx >= 0) _viewportChangeHandlers.splice(idx, 1);
      if (_cy) { try { _cy.off('zoom pan resize', handler); } catch (_) {} }
    };
  }

  function _onSelectionChanged(handler) {
    if (typeof handler !== 'function') return function () {};
    _selectionChangeHandlers.push(handler);
    if (_cy) { try { _cy.on('select unselect', handler); } catch (_) {} }
    return function () {
      var idx = _selectionChangeHandlers.indexOf(handler);
      if (idx >= 0) _selectionChangeHandlers.splice(idx, 1);
      if (_cy) { try { _cy.off('select unselect', handler); } catch (_) {} }
    };
  }

  // ── D37j — Client-side authority-context view ─────────────────────────
  //
  // Operator question: "What is the authority context of this object?"
  //
  // When the operator selects a supported governance object and
  // clicks the toolbar's `View authority context` control, the
  // renderer hides every Cytoscape element that is not in the
  // node's directed-traversal authority context (predecessors ∪
  // successors ∪ self), plus the BS-default fail-mode-policy edge
  // for any business_service in the focus (inherited policy
  // semantics). The fit camera is then constrained to the visible
  // collection via the existing safe-area-aware `_fitToAvailableCanvas`,
  // which already filters to `cy.elements(':visible')`.
  //
  // The HTML card overlay mirrors visibility through the D37j
  // `_syncCards` extension (`n.visible() → card.style.display`).
  //
  // Eligible focal kinds in D37j:
  //   • decision_surface  — always inside the loaded BS, complete
  //   • authority_profile — same; profile is bound to one surface
  //   • authority_grant   — drilldown from profile/surface
  //
  // Disabled focal kinds in D37j (the loaded BS-rooted graph cannot
  // represent the full context, so a client-side view would be
  // misleading; deferred to D38a/b/c backend re-rooting):
  //   • business_service  — already the default root
  //   • agent             — cross-BS authorisation
  //   • fail_mode_policy  — cross-BS policy applicability
  //   • escalation_target — cross-BS escalation references
  //
  // The view is purely visual: no projection re-fetch, no graph
  // mutation, no element removal. Exit restores all elements via
  // `cy.elements().show()`.
  var _AUTHORITY_CONTEXT_ELIGIBLE_KINDS = {
    decision_surface:  true,
    authority_profile: true,
    authority_grant:   true,
  };

  var _authorityContextActive          = false;
  var _authorityContextFocalNodeId     = '';
  var _authorityContextFocalKind       = '';
  var _authorityContextChangeHandlers  = [];

  // _isAuthorityContextEligibleKind — single source of truth for
  // which focal kinds D37j enables. The test suite pins this map
  // to exactly the three supported kinds and bans the others.
  function _isAuthorityContextEligibleKind(kind) {
    if (!kind) return false;
    return _AUTHORITY_CONTEXT_ELIGIBLE_KINDS[kind] === true;
  }

  // _getEligibleSelectedNode returns the single selected node IF
  // it is eligible for authority-context view, else null. Multi-
  // selection and unsupported kinds both return null so the toolbar
  // can disable the control without an ambiguous reason.
  function _getEligibleSelectedNode(cy) {
    if (!cy || typeof cy.elements !== 'function') return null;
    var sel;
    try { sel = cy.elements(':selected'); } catch (_) { return null; }
    if (!sel || typeof sel.length !== 'number' || sel.length !== 1) return null;
    var n = sel[0];
    if (!n || typeof n.data !== 'function') return null;
    var kind = String(n.data('kind') || '');
    if (!_isAuthorityContextEligibleKind(kind)) return null;
    return n;
  }

  function _canViewAuthorityContext() {
    return _getEligibleSelectedNode(_cy) !== null;
  }

  function _isAuthorityContextActive() {
    return _authorityContextActive === true;
  }

  // _computeAuthorityContext returns the focus collection (nodes +
  // edges) for an authority-context view rooted at `node`. The walk
  // is directed: predecessors ∪ successors ∪ self. Additionally,
  // any business_service in the predecessors path is extended with
  // its business_service_has_fail_mode_policy edge → policy node
  // so the inherited BS-default policy stays visible alongside the
  // ancestor BS. Cytoscape's predecessors() / successors() each
  // return both nodes and the path edges, so no extra edge-collection
  // step is needed beyond the BS-default policy addition.
  function _computeAuthorityContext(cy, node) {
    if (!cy || !node) return null;
    if (typeof node.predecessors !== 'function' ||
        typeof node.successors   !== 'function') return null;
    var focus;
    try {
      focus = node.predecessors().union(node.successors()).union(node);
    } catch (_) { return null; }
    if (!focus) return null;
    // Inherited BS-default fail-mode-policy semantics: for each
    // business_service ancestor in the focus, include its outgoing
    // business_service_has_fail_mode_policy edge + target node.
    try {
      var bsNodes = focus.filter('node[kind = "business_service"]');
      if (bsNodes && bsNodes.length) {
        for (var i = 0; i < bsNodes.length; i++) {
          var bs = bsNodes[i];
          if (typeof bs.outgoers !== 'function') continue;
          var defaultEdges = bs.outgoers('edge[kind = "business_service_has_fail_mode_policy"]');
          if (defaultEdges && defaultEdges.length) {
            focus = focus.union(defaultEdges);
            if (typeof defaultEdges.targets === 'function') {
              focus = focus.union(defaultEdges.targets());
            }
          }
        }
      }
    } catch (_) { /* swallow — BS-default extension is best-effort */ }
    return focus;
  }

  function _emitAuthorityContextChange() {
    var handlers = _authorityContextChangeHandlers.slice();
    for (var i = 0; i < handlers.length; i++) {
      try { handlers[i](); } catch (_) { /* swallow per-handler errors */ }
    }
  }

  // _viewAuthorityContext — enter context view for the currently
  // selected eligible node. No-op when no eligible node is selected.
  // Hides non-focus cy elements via `.hide()` (Cytoscape culls edges
  // with hidden endpoints, so no edge bookkeeping is required), then
  // calls `_syncCards()` to mirror visibility onto the HTML overlay,
  // and fits camera to the visible (focus) collection.
  function _viewAuthorityContext() {
    if (!_cy) return false;
    var node = _getEligibleSelectedNode(_cy);
    if (!node) return false;
    var focus = _computeAuthorityContext(_cy, node);
    if (!focus || focus.length === 0) return false;
    var nonFocus;
    try { nonFocus = _cy.elements().difference(focus); } catch (_) { return false; }
    try { nonFocus.hide(); } catch (_) { /* swallow */ }
    _authorityContextActive      = true;
    _authorityContextFocalNodeId = String(node.data('id') || '');
    _authorityContextFocalKind   = String(node.data('kind') || '');
    _syncCards();
    try { _fitToAvailableCanvas(_cy); } catch (_) { /* swallow */ }
    _emitAuthorityContextChange();
    return true;
  }

  // _exitAuthorityContext — restore the full visible graph. Safe to
  // call when context is not active (idempotent). Always shows every
  // cy element so prior `.hide()` is reversed; clears state and
  // re-syncs card visibility. Does NOT auto-fit (caller decides
  // whether the camera should jump back).
  function _exitAuthorityContext() {
    if (!_authorityContextActive) return false;
    if (_cy && typeof _cy.elements === 'function') {
      try { _cy.elements().show(); } catch (_) { /* swallow */ }
    }
    _authorityContextActive      = false;
    _authorityContextFocalNodeId = '';
    _authorityContextFocalKind   = '';
    _syncCards();
    _emitAuthorityContextChange();
    return true;
  }

  function _toggleAuthorityContext() {
    return _authorityContextActive ? _exitAuthorityContext() : _viewAuthorityContext();
  }

  function _onAuthorityContextChanged(handler) {
    if (typeof handler !== 'function') return function () {};
    _authorityContextChangeHandlers.push(handler);
    return function () {
      var idx = _authorityContextChangeHandlers.indexOf(handler);
      if (idx >= 0) _authorityContextChangeHandlers.splice(idx, 1);
    };
  }

  // _checkAutoExitContext — Option A from the D37j brief: when the
  // operator changes the selection to a node that is NOT the current
  // focal node, the context view auto-exits. Bound to `cy.on('select',
  // 'node', ...)` in `_wireInteractions`. The unselect event is
  // intentionally not used as the trigger: an empty selection is a
  // valid state inside context view (operator may un-select to view
  // the surrounding context without losing the focus).
  function _checkAutoExitContext() {
    if (!_authorityContextActive || !_cy) return;
    var sel;
    try { sel = _cy.elements(':selected'); } catch (_) { return; }
    if (!sel || sel.length === 0) return;
    if (sel.length === 1) {
      var selId = String(sel[0].data('id') || '');
      if (selId === _authorityContextFocalNodeId) return;
    }
    _exitAuthorityContext();
  }

  // ── D33x-list-mode — Cytoscape-backed List Mode ──────────────────────
  //
  // List Mode arranges the Authority Graph's nodes into a columnar
  // list inside the same Cytoscape instance. Edges are hidden;
  // selection still routes to the production right drawer via the
  // existing tap-handler + carrier-DOM contract; the Fit + zoom
  // toolbar buttons remain functional.
  //
  // The mode is toggled by the workbench Form / Records button when
  // the active lens is `authority` and `body.cytoscape-poc-active`
  // is set (see the lens-aware branch in index.html). Outside that
  // combination the button keeps its legacy behaviour
  // (`MIDASExplorerServices.showRecord`).
  //
  // Group order (top to bottom, then column to column):
  //   1. business_service   (root is sorted first within this group)
  //   2. decision_surface
  //   3. authority_profile
  //   4. authority_grant
  //   5. agent
  //   6. fail_mode_policy
  //   7. escalation_target
  //   8. anything else      (sorted alphabetically by kind)
  //
  // Within each group, nodes sort by `label`, then `id`, with the
  // single `isRoot:true` node pinned to the top of its bucket.
  //
  // Column wrapping:
  //   - Estimate the number of rows that fit vertically inside the
  //     usable canvas (`cy.height()` minus a safe-area buffer).
  //   - Compute the minimum column count needed to fit every node
  //     (capped at LIST_MAX_COLUMNS = 4).
  //   - Distribute rows evenly across that column count so columns
  //     stay roughly balanced.
  //   - Group transitions cost an extra "blank row" so groups read
  //     as distinct visual bands; the root is always the first
  //     placement in column 0.
  var LIST_GROUP_ORDER = [
    'business_service',
    'decision_surface',
    'authority_profile',
    'authority_grant',
    'agent',
    'fail_mode_policy',
    'escalation_target',
  ];
  var LIST_ROW_PITCH      = 80;   // vertical distance between node centres (px)
  var LIST_COL_PITCH      = 360;  // horizontal distance between column centres (px)
  var LIST_TOP_PAD        = 60;   // top padding inside the cy mount (px)
  var LIST_LEFT_PAD       = 60;   // left padding inside the cy mount (px)
  var LIST_GROUP_GAP_ROWS = 1;    // blank rows inserted between groups
  var LIST_MAX_COLUMNS    = 4;
  var LIST_MIN_ROWS_PER_COL = 4;  // never collapse below this even on tiny viewports

  // _computeListPositions returns `{ positions, columnCount,
  // rowsPerColumn, ordered }`. Pure relative to `cy` + `availableH`;
  // exposed on the public surface so tests can assert ordering and
  // wrapping without instantiating Cytoscape.
  function _computeListPositions(cy, availableH) {
    var out = { positions: {}, columnCount: 1, rowsPerColumn: 0, ordered: [] };
    if (!cy || typeof cy.nodes !== 'function') return out;

    // Bucket by kind.
    var buckets = {};
    cy.nodes().forEach(function (n) {
      var k = String(n.data('kind') || 'other');
      (buckets[k] = buckets[k] || []).push(n);
    });

    // Sort each bucket: root first, then by label, then id.
    function _sortInGroup(a, b) {
      var ar = a.data('isRoot') ? 0 : 1;
      var br = b.data('isRoot') ? 0 : 1;
      if (ar !== br) return ar - br;
      var al = String(a.data('label') || a.data('name') || a.data('id') || '').toLowerCase();
      var bl = String(b.data('label') || b.data('name') || b.data('id') || '').toLowerCase();
      if (al < bl) return -1;
      if (al > bl) return 1;
      var ai = String(a.data('id') || '');
      var bi = String(b.data('id') || '');
      if (ai < bi) return -1;
      if (ai > bi) return 1;
      return 0;
    }

    // Build ordered group list in the documented order, then append
    // any leftover kinds alphabetically so unknown future kinds
    // still get a deterministic place.
    var ordered = [];
    for (var i = 0; i < LIST_GROUP_ORDER.length; i++) {
      var k = LIST_GROUP_ORDER[i];
      if (buckets[k]) {
        var arr = buckets[k].slice().sort(_sortInGroup);
        ordered.push({ kind: k, nodes: arr });
        delete buckets[k];
      }
    }
    var leftover = Object.keys(buckets).sort();
    for (var j = 0; j < leftover.length; j++) {
      var arr2 = buckets[leftover[j]].slice().sort(_sortInGroup);
      ordered.push({ kind: leftover[j], nodes: arr2 });
    }

    // Count effective rows (nodes + group gaps between successive
    // groups). The first group does not get a leading gap.
    var effectiveRows = 0;
    for (var g = 0; g < ordered.length; g++) {
      if (g > 0) effectiveRows += LIST_GROUP_GAP_ROWS;
      effectiveRows += ordered[g].nodes.length;
    }
    if (effectiveRows === 0) return out;

    // Resolve available rows from the canvas height. `availableH`
    // is optional; if omitted, fall back to `cy.height()`.
    var ch = 0;
    if (typeof availableH === 'number' && availableH > 0) {
      ch = availableH;
    } else if (typeof cy.height === 'function') {
      ch = cy.height();
    }
    var usableH = Math.max(0, ch - LIST_TOP_PAD - 24);
    var rowsPerCol = Math.max(LIST_MIN_ROWS_PER_COL, Math.floor(usableH / LIST_ROW_PITCH));

    // Minimum column count that fits effectiveRows, capped at
    // LIST_MAX_COLUMNS. If the data genuinely can't fit, we still
    // allow vertical overflow inside the last column (Cytoscape pans
    // freely; we just prefer to avoid it).
    var cols = Math.max(1, Math.ceil(effectiveRows / rowsPerCol));
    if (cols > LIST_MAX_COLUMNS) cols = LIST_MAX_COLUMNS;
    // Re-balance: rowsPerCol = ceil(effectiveRows / cols) so columns
    // are evenly filled rather than the last column being sparse.
    var perCol = Math.ceil(effectiveRows / cols);

    var positions = {};
    var col = 0;
    var row = 0;
    for (var gi = 0; gi < ordered.length; gi++) {
      var group = ordered[gi];
      if (gi > 0) {
        // Insert a blank row before each non-first group. If the
        // blank row would fall in the last position of a column,
        // flush to the next column so the group header lines up at
        // the top — but never split the very first group (root
        // stays in column 0).
        if (gi > 1 && row + LIST_GROUP_GAP_ROWS >= perCol && col < cols - 1) {
          col++;
          row = 0;
        } else {
          row += LIST_GROUP_GAP_ROWS;
        }
      }
      for (var ni = 0; ni < group.nodes.length; ni++) {
        if (row >= perCol && col < cols - 1) {
          col++;
          row = 0;
        }
        positions[group.nodes[ni].id()] = {
          x: LIST_LEFT_PAD + col * LIST_COL_PITCH,
          y: LIST_TOP_PAD  + row * LIST_ROW_PITCH,
        };
        row++;
      }
    }

    out.positions     = positions;
    out.columnCount   = cols;
    out.rowsPerColumn = perCol;
    out.ordered       = ordered;
    return out;
  }

  // applyListLayout snapshots current graph-mode positions (once)
  // then arranges nodes columnarly and hides edges. Idempotent:
  // calling while already in list mode just recomputes positions
  // (e.g. after a resize).
  function applyListLayout() {
    if (!_cy) return;
    try {
      // Snapshot positions BEFORE we move nodes, so switching back
      // to graph mode restores the spine layout faithfully.
      if (!_savedGraphPositions) {
        _savedGraphPositions = {};
        _cy.nodes().forEach(function (n) {
          var p = n.position();
          if (p && typeof p.x === 'number' && typeof p.y === 'number') {
            _savedGraphPositions[n.id()] = { x: p.x, y: p.y };
          }
        });
      }
      var ch = (typeof _cy.height === 'function') ? _cy.height() : 0;
      var plan = _computeListPositions(_cy, ch);
      _cy.batch(function () {
        // Hide edges. removeStyle() on graph-mode restore returns
        // them to their stylesheet defaults (visible).
        _cy.edges().style('display', 'none');
        // Apply preset positions to every node we have a position
        // for. Nodes outside `positions` (none in practice) keep
        // their current position.
        Object.keys(plan.positions).forEach(function (id) {
          var n = _cy.getElementById(id);
          if (n && n.length) n.position(plan.positions[id]);
        });
      });
      _viewMode = 'list';
      // Refit so the new column layout lands inside the visible
      // region (same per-side inset model as the initial fit).
      _fitToAvailableCanvas(_cy, { elements: _cy.nodes() });
    } catch (_) { /* swallow — never break the active session */ }
  }

  // applyGraphLayout restores the snapshot taken when list mode was
  // entered and unhides edges. If there is no snapshot (list mode
  // was never entered), it just unhides edges so the call is a safe
  // no-op for the graph-mode steady state.
  function applyGraphLayout() {
    if (!_cy) return;
    try {
      _cy.batch(function () {
        _cy.edges().removeStyle('display');
        if (_savedGraphPositions) {
          Object.keys(_savedGraphPositions).forEach(function (id) {
            var n = _cy.getElementById(id);
            if (n && n.length) n.position(_savedGraphPositions[id]);
          });
        }
      });
      _viewMode = 'graph';
      _savedGraphPositions = null;
      _fitToAvailableCanvas(_cy);
    } catch (_) { /* swallow */ }
  }

  // setViewMode is the public toggle. Accepts 'graph' or 'list'.
  // Other inputs are silently ignored. Returns the resolved mode
  // so callers can confirm what was applied.
  function setViewMode(mode) {
    if (mode !== 'graph' && mode !== 'list') return _viewMode;
    if (mode === _viewMode) {
      // Idempotent: re-apply so a resize / drawer toggle that
      // happened between calls still gets a fresh fit.
      if (mode === 'list') applyListLayout();
      else                 applyGraphLayout();
      return _viewMode;
    }
    if (mode === 'list') applyListLayout();
    else                 applyGraphLayout();
    return _viewMode;
  }

  function getViewMode() {
    return _viewMode;
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

  // D33a-impl-1 — Refit when legend / inspector expand state changes
  // so the graph never ends up underneath a newly-expanded chrome
  // surface.
  // D33x-fit-zoom-root — Delegates to `_fitToAvailableCanvas` so the
  // per-side insets are honoured (matches the initial fit + the
  // toolbar Fit button + the drawer/resize observers).
  function _refitWithSafeArea() {
    if (!_cy) return;
    try {
      _cy.resize();
      _fitToAvailableCanvas(_cy);
    } catch (_) { /* swallow */ }
  }

  // D35d-port-authority-cytoscape-to-graphviewport — renderer-host
  // bridge state. `_rendererCtx` is the ctx object the GraphViewport
  // host passed to `factory.mount(slotEl, ctx)`. It is consulted by
  // `_safeAreaPadding` (for fit-padding composition) and stored so the
  // factory's destroy can unsubscribe from `ctx.onResize`.
  // `_rendererResizeUnsub` is the unsubscribe function returned by
  // ctx.onResize. Both are cleared in the factory's destroy.
  var _rendererCtx         = null;
  var _rendererResizeUnsub = null;

  // D35d — `_authorityRendererFactory` implements the
  // MIDASGraphRendererFactory contract published by graph-viewport.js.
  // It is the bridge between the GraphViewport host's activation
  // lifecycle and the existing PoC internals (`_mountEl`, `_destroyCy`,
  // `_renderPayload`, `_refitWithSafeArea`).
  //
  // mount(slotEl, ctx):
  //   • Creates the `.cytoscape-poc-mount` element as a CHILD of
  //     `slotEl` (the `.midas-graph-renderer-slot` element supplied by
  //     the host). This replaces the pre-D35d behaviour of inserting
  //     the mount before `#gmap-canvas` inside `.governance-map-
  //     canvas-scroll` — the host's slot is now the strategic mount
  //     parent.
  //   • Stores ctx for later use by `_safeAreaPadding` and
  //     `_uninstallPoc`.
  //   • Subscribes to `ctx.onResize(...)` so window/drawer-induced
  //     resize triggers `_refitWithSafeArea` (calls `cy.resize()` +
  //     `_fitToAvailableCanvas`). Returns the unsubscribe.
  //   • Does NOT call cy initialisation directly — that runs in
  //     `_renderPayload` (lens-driven). The factory just owns the
  //     mount root and the resize lifecycle.
  //
  // destroy():
  //   • Unsubscribes the resize handler.
  //   • Calls `_teardownPocResources()` to destroy cy + remove mount
  //     DOM + clear PoC state. Does NOT call `_uninstallPoc` (which
  //     would recurse into the host's `deactivate`).
  //   • Clears `_rendererCtx` so a subsequent activate starts clean.
  var _authorityRendererFactory = {
    mount: function (slotEl, ctx) {
      _rendererCtx = ctx || null;
      // If a prior mount survived, remove it before creating the new
      // one. Idempotent: a re-activation cleans the slot of stale
      // Authority DOM without touching anything else in the slot.
      if (_mountEl && _mountEl.parentNode) {
        try { _mountEl.parentNode.removeChild(_mountEl); }
        catch (_) { /* swallow */ }
      }
      _mountEl = document.createElement('div');
      _mountEl.id = 'gmap-cytoscape-mount';
      _mountEl.className = 'cytoscape-poc-mount';
      _mountEl.setAttribute('role', 'application');
      // D37b — production aria-label. The pre-D37b "(Cytoscape PoC)"
      // suffix is retired; the user-facing renderer name is just
      // "Authority Graph" because this is the production Authority
      // renderer.
      _mountEl.setAttribute('aria-label', 'Authority Graph');
      try { slotEl.appendChild(_mountEl); } catch (_) { /* swallow */ }
      // Subscribe to host resize. The factory owns only the
      // unsubscribe; the host owns the underlying ResizeObserver.
      if (ctx && typeof ctx.onResize === 'function') {
        try { _rendererResizeUnsub = ctx.onResize(_refitWithSafeArea); }
        catch (_) { _rendererResizeUnsub = null; }
      }
      return {
        destroy: function () {
          try { if (_rendererResizeUnsub) _rendererResizeUnsub(); }
          catch (_) { /* swallow */ }
          _rendererResizeUnsub = null;
          _teardownPocResources();
          _rendererCtx = null;
        },
      };
    },
  };

  function _ensureMount() {
    if (_mountEl && _mountEl.isConnected) return _mountEl;

    // D35d + D37b — Route mount creation through the GraphViewport
    // host (the production path post-D35a/D35b/D35c). The host's
    // `activateById('authority')` (renderer id was `'authority-
    // cytoscape'` pre-D37b; promoted to the production id `'authority'`
    // in D37b) deactivates the previous renderer (typically
    // 'native-context' — its destroy is a no-op so native DOM
    // survives), then calls our factory's mount which creates
    // `_mountEl` inside `.midas-graph-renderer-slot`. Subsequent
    // calls during the same activation short-circuit
    // via the `_mountEl.isConnected` guard above.
    var vp = (window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport) || null;
    if (vp &&
        typeof vp.activateById === 'function' &&
        typeof vp.getActiveRendererId === 'function' &&
        typeof vp.getRendererSlotEl === 'function' &&
        vp.getRendererSlotEl()) {
      if (vp.getActiveRendererId() !== 'authority') {
        // D35g + D37b — Activate by registered id `'authority'` (factory
        // is already in the host's registry — see the IIFE-end
        // registration block at the bottom of this module). Falls back
        // to false if the renderer isn't registered, which is a
        // fail-safe: install bails out.
        try { vp.activateById('authority'); }
        catch (_) { /* swallow — install bails below */ }
      } else {
        // Already active but mount was disconnected (e.g. cleared by
        // a stray DOM op). Re-run the factory's mount to rebuild.
        try {
          var slotEl = vp.getRendererSlotEl();
          _authorityRendererFactory.mount(slotEl, _rendererCtx || {
            viewportEl:      vp.getViewportEl && vp.getViewportEl(),
            slotEl:          slotEl,
            getViewportRect: vp.getViewportRect,
            getSafeArea:     vp.getSafeArea,
            onResize:        vp.onResize,
            hooks: (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || null,
          });
        } catch (_) { /* fall through to legacy fallback */ }
      }
      if (_mountEl) return _mountEl;
    }

    // D35f-retire-transitional-renderer-debt — Legacy fallback
    // REMOVED. Pre-D35f, when the GraphViewport host was absent the
    // PoC would fall back to inserting `.cytoscape-poc-mount` before
    // `#gmap-canvas` inside `.governance-map-canvas-scroll`. That
    // path was a transitional bridge for pre-D35a/D35b builds and
    // headless harnesses without graph-viewport.js. Every shipped
    // Explorer now loads the host BEFORE this module
    // (graph-viewport.js precedes graph-renderer.js in index.html),
    // so the host is always available. If the host is somehow
    // absent, install fails safely (returns null mount) rather than
    // building a parallel architecture.
    _mountEl = null;
    return null;
  }

  // D33x-left-poc-panel — `_setLegendExpanded`, `_renderLegend`, and
  // `_legendRow` retired along with the floating left "Authority
  // Graph" PoC panel. No replacement: the panel was redundant with
  // the MIDAS Posture & Help drawer tab.
  // D33x-list-mode — `_renderInspectorEmpty`, `_renderInspector`,
  // and `_wireInspectorToggle` retired. The floating PoC card they
  // composed has been removed; selected-node detail is rendered by
  // the production right drawer (`#gmap-details`) via the carrier
  // DOM contract (see `_renderInspectorCarriers` below).

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
    // D34a-cytoscape-html-overlay-spike — Tear down the MIDAS-rich
    // HTML overlay before Cytoscape destroy. No-op when the spike
    // gate is closed.
    try {
      var _htmlOverlayMod = window.MIDASExplorerGraph && window.MIDASExplorerGraph.cytoscapeHtmlOverlay;
      if (_htmlOverlayMod && typeof _htmlOverlayMod.destroy === 'function') {
        _htmlOverlayMod.destroy();
      }
    } catch (_) { /* swallow */ }
    // D33a-spike-2g-impl-5 — Drop the inspector carriers before
    // Cytoscape teardown so successive renders cannot accumulate
    // stale `.gmap-node` carriers under #gmap-canvas. Idempotent.
    _clearInspectorCarriers();
    if (_cy) {
      try { _cy.destroy(); } catch (_) { /* swallow */ }
      _cy = null;
    }
    // D33x-list-mode — Reset view-mode state across teardown so a
    // fresh render starts in `graph` with no stale position snapshot.
    _viewMode = 'graph';
    _savedGraphPositions = null;
  }

  // D35d — `_teardownPocResources` does the actual DOM/cy cleanup
  // work, extracted from the pre-D35d `_uninstallPoc`. The split
  // exists because the GraphViewport host's `deactivate()` calls the
  // factory's destroy, which calls this helper. `_uninstallPoc` now
  // routes through the host (if active) so the host's active-renderer
  // state is cleared too — but it must NOT call `_teardownPocResources`
  // directly in that case, because the factory's destroy will do it.
  //
  // Strictly Authority-owned cleanup (no body-class flip — that
  // remains transitional debt in `_uninstallPoc`).
  function _teardownPocResources() {
    _destroyHtmlCardOverlay();
    _clearInspectorCarriers();
    _destroyCy();
    if (_mountEl && _mountEl.parentNode) {
      _clearOverlays(_mountEl);
      _mountEl.parentNode.removeChild(_mountEl);
    }
    _mountEl = null;
    _viewMode = 'graph';
    _savedGraphPositions = null;
  }

  // D33a-impl-1 — Full PoC teardown. Used by the public _uninstall
  // surface and the GraphViewport factory destroy path (D37p-clean-1
  // retired the dead `lensImpl.clear` route that used to call this).
  // Releases Cytoscape, removes every
  // PoC-owned DOM child (legend, inspector, unavailable overlays,
  // mount itself), and clears the body class so disabling the PoC
  // leaves zero residue. Idempotent: repeated calls are safe.
  //
  // D35d — Routes through the GraphViewport host's `deactivate()`
  // when the host owns our activation. This ensures the host's
  // active-renderer state is cleared and the factory's destroy runs
  // (which calls `_teardownPocResources`). If the host is unavailable
  // or doesn't own our activation, falls back to direct cleanup.
  //
  // Body-class removal is TRANSITIONAL DEBT scheduled for D35f.
  // The `body.cytoscape-poc-active` class drives load-bearing CSS
  // (most notably `body.cytoscape-poc-active #gmap-canvas { display:
  // none !important }`) that the host's renderer-id model is meant
  // to replace. Until every cy-based renderer is host-owned and the
  // CSS migrates to a renderer-id attribute, the class stays.
  function _uninstallPoc() {
    // D33a-spike-2g-impl-5 — Belt-and-braces clear of any inspector
    // carrier DOM the PoC dropped under #gmap-canvas. _destroyCy
    // (reached via _teardownPocResources below OR via the factory's
    // destroy when the host routes through deactivate) also calls
    // _clearInspectorCarriers; the duplicate call here keeps
    // _uninstallPoc idempotent if invoked outside the normal render
    // path (test teardown, manual diagnostics). The carrier clear
    // is itself idempotent so the double call is harmless.
    _clearInspectorCarriers();

    // D35d + D37b — Route the teardown through the GraphViewport host
    // when it owns our activation. The host's deactivate() calls the
    // factory's destroy, which calls _teardownPocResources, which
    // calls _destroyCy + _clearInspectorCarriers + mount removal.
    // If the host is unavailable OR 'authority' isn't the active id,
    // fall back to direct teardown.
    var vp = (window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport) || null;
    var routedThroughHost = false;
    if (vp &&
        typeof vp.deactivate === 'function' &&
        typeof vp.getActiveRendererId === 'function' &&
        vp.getActiveRendererId() === 'authority') {
      try { vp.deactivate(); routedThroughHost = true; }
      catch (_) { routedThroughHost = false; }
    }
    if (!routedThroughHost) {
      _teardownPocResources();
    }
    // D35f-retire-transitional-renderer-debt — Body-class removal
    // RETIRED. The host's `viewport.deactivate()` (called via
    // routedThroughHost above) clears the
    // `data-active-renderer="authority"` attribute on
    // `.midas-graph-viewport` (or restores it to the baseline
    // 'native-context'), which is now the sole source of strategic
    // renderer-state CSS keying. The pre-D35f
    // `body.cytoscape-poc-active` flip is gone.
  }

  // ── D33a-spike-2 + D37f — Authority HTML-card overlay (production) ────
  //
  // The Authority HTML-card overlay is the PRODUCTION Authority
  // visual as of D37f. Cytoscape continues to own layout, edges,
  // hover, drag, pan, zoom, and hit-testing; the overlay is purely
  // presentational. Overlay + cards are `pointer-events: none` so
  // every tap falls through to Cytoscape's canvas.
  //
  // Projection model: D34i two-tier (lifted from the Context spike's
  // canonical implementation pinned by D35e tests).
  //
  //   • LAYER tier: cy.pan + cy.zoom → ONE transform on the overlay
  //     element. Updated on `pan zoom render resize` events. Cost
  //     O(1) per event regardless of node count.
  //   • CARDS tier: cy.node.position() (MODEL coords) → ONE transform
  //     per card via `translate(model.x, model.y) translate(-50%, -50%)`.
  //     Updated on `position bounds layoutstop add select unselect`
  //     events. Also mirrors `n.selected()` onto each card's
  //     `.selected` class.
  //
  // The layer's `scale(cy.zoom())` projects every card from model
  // space to rendered space, so per-card transforms remain in model
  // coordinates (no `renderedPosition` and no per-card `scale`).
  // `transform-origin: top left` on the overlay (declared in
  // authority-cytoscape-poc.css) MUST match Cytoscape's projection
  // origin so `cy.pan(0,0)` lands at the same screen pixel.
  //
  // Each tier has its own rAF coalescing flag so a burst of
  // pan/zoom collapses to one layer write per frame and a burst of
  // drag-position events collapses to one card-tier write per frame.
  //
  // The overlay container, card elements, and both event listeners
  // are torn down by _destroyHtmlCardOverlay. Wired into both
  // _destroyCy (per-render teardown) and _uninstallPoc (full PoC
  // teardown) so service refresh and lens unmount both leave a clean
  // DOM.
  //
  // Pre-D37f the install was gated on `_activeTheme === 'html-card'`
  // and the sync was a single-tier `renderedPosition` projection.
  // D37f retired the theme-gate so HTML cards render by default, and
  // adopted the proven D34i two-tier model.

  // D34i two-tier event constants. Authority mirrors the Context
  // spike's wiring verbatim.
  var LAYER_SYNC_EVENTS = 'pan zoom render resize';
  var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect';
  var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position';

  var _htmlOverlayEl    = null;   // <div> containing cards
  var _htmlCardsByKey   = {};     // refKey -> <article> element
  // Per-tier rAF flags + bound handlers. Each tier coalesces
  // independently so the two cannot starve each other.
  var _syncLayerRaf     = 0;
  var _syncCardsRaf     = 0;
  var _syncLayerBound   = null;
  var _syncCardsBound   = null;

  // D37k-impl-1 — HTML edge-label overlay (single shared chip).
  //
  // A new overlay tier above the cards overlay. One shared chip
  // element is positioned in model coordinates at the hovered
  // edge's midpoint and projected via the same `_syncLayer` pan/
  // zoom transform that drives the cards overlay (so the chip
  // tracks the cy viewport with no separate transform pipeline).
  //
  // The chip is hover-only: shown by `_focusEdge` (called from the
  // existing `mouseover` 'edge' handler), hidden by
  // `_clearInteractionState` (called from `mouseout` /
  // `_focusNode` / background tap). No per-edge DOM — one chip,
  // re-positioned on each hover. The cards-tier sync re-positions
  // the chip when its edge's endpoints move (e.g. node drag).
  var _edgeLabelOverlayEl     = null;
  var _edgeLabelChipEl        = null;
  var _edgeLabelFocusedEdgeId = '';

  // _syncLayer — applies cy.pan + cy.zoom to the overlay element as
  // ONE transform with `transform-origin: top left`. After this
  // write, every descendant card is implicitly projected from model
  // space to rendered space, including scale. Pan/zoom events bind
  // here, so a burst of pan/zoom during user interaction collapses
  // to one style write per frame (rAF-coalesced via `_syncLayerBound`).
  //
  // MUST NOT iterate `_htmlCardsByKey` — pan/zoom must cost O(1)
  // per event regardless of node count. Pinned by D37f tests.
  function _syncLayer() {
    if (!_cy || !_htmlOverlayEl) return;
    var pan, zoom;
    try {
      pan  = (typeof _cy.pan  === 'function') ? _cy.pan()  : { x: 0, y: 0 };
      zoom = (typeof _cy.zoom === 'function') ? _cy.zoom() : 1;
    } catch (_) { return; }
    if (!pan || typeof zoom !== 'number') return;
    var t = 'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')';
    var stl = _htmlOverlayEl.style;
    stl.transformOrigin = 'top left';
    stl.webkitTransform = t;
    stl.transform       = t;
    // D37k-impl-1 — Project the edge-label overlay through the same
    // pan/zoom transform as the cards overlay. The chip inside is
    // positioned in model coordinates, so applying the layer
    // transform here keeps the chip pinned to the hovered edge's
    // midpoint across pan/zoom. Two style writes (cards overlay +
    // edge overlay) — still O(1) per event, no card iteration.
    if (_edgeLabelOverlayEl) {
      var els = _edgeLabelOverlayEl.style;
      els.transformOrigin = 'top left';
      els.webkitTransform = t;
      els.transform       = t;
    }
  }

  // _syncCards — writes one transform per card in MODEL coordinates.
  // The per-card transform centres the card on the model position via
  // `translate(-50%, -50%)`. NO scale here — the layer's scale does
  // the projection. Also mirrors cy selected state onto each card's
  // `.selected` class.
  //
  // Bound to `CARDS_SYNC_EVENTS` (position / bounds / layoutstop /
  // add / select / unselect). Cy fires `position` per-frame during
  // a node drag, so this re-runs per frame during drag — but the
  // body is one style write per card, no DOM reflow, and is rAF-
  // coalesced via `_syncCardsBound`.
  //
  // MUST NOT use `renderedPosition` or per-card `scale(` — the layer
  // already projects.
  function _syncCards() {
    if (!_cy || !_htmlCardsByKey) return;
    var keys = Object.keys(_htmlCardsByKey);
    for (var i = 0; i < keys.length; i++) {
      var id = keys[i];
      var card = _htmlCardsByKey[id];
      if (!card) continue;
      var n = _cy.$id(id);
      if (!n || !n.length) {
        card.style.display = 'none';
        continue;
      }
      // D37j — Mirror Cytoscape visibility onto the HTML card so an
      // authority-context view that hides non-focus cy nodes also
      // hides their card overlays. Without this mirror, a hidden cy
      // node would leave a floating card with no underlying node.
      // The check is defensive (`typeof n.visible === 'function'`)
      // so a future fixture that doesn't implement `visible` cannot
      // break the cards tier.
      if (typeof n.visible === 'function' && !n.visible()) {
        card.style.display = 'none';
        continue;
      }
      var p = n.position();
      var t = 'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)';
      card.style.webkitTransform = t;
      card.style.transform       = t;
      card.style.display = '';
      if (n.selected()) card.classList.add('selected');
      else              card.classList.remove('selected');
    }
    // D37k-impl-1 — Keep the hovered edge-label chip aligned when a
    // node drag moves an edge endpoint. `position` events on nodes
    // already fire this cards-tier sync, so the chip re-positions
    // for free under the same rAF coalescing. Idempotent + no-op
    // when no chip is shown.
    _syncEdgeLabelPosition();
  }

  function _installHtmlCardOverlay(cy, mount, elements) {
    if (!cy || !mount) return;
    _destroyHtmlCardOverlay();

    _htmlOverlayEl = document.createElement('div');
    _htmlOverlayEl.className = 'cytoscape-poc-html-overlay';
    _htmlOverlayEl.setAttribute('role', 'presentation');
    // D37f — Cards are presentational; screen readers read node
    // details from the right drawer when a node is selected.
    _htmlOverlayEl.setAttribute('aria-hidden', 'true');
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

    // D34i two-tier event bindings. Each tier coalesces via its own
    // rAF flag so a burst of pan/zoom collapses to one layer write
    // per frame and a burst of drag position events collapses to one
    // card-tier write per frame.
    _syncLayerBound = function () {
      if (_syncLayerRaf) return;
      if (typeof window.requestAnimationFrame === 'function') {
        _syncLayerRaf = window.requestAnimationFrame(function () {
          _syncLayerRaf = 0;
          _syncLayer();
        });
      } else {
        _syncLayer();
      }
    };
    _syncCardsBound = function () {
      if (_syncCardsRaf) return;
      if (typeof window.requestAnimationFrame === 'function') {
        _syncCardsRaf = window.requestAnimationFrame(function () {
          _syncCardsRaf = 0;
          _syncCards();
        });
      } else {
        _syncCards();
      }
    };
    try { cy.on(LAYER_SYNC_EVENTS, _syncLayerBound); } catch (_) { /* swallow */ }
    try { cy.on(CARDS_SYNC_EVENTS, _syncCardsBound); } catch (_) { /* swallow */ }

    // Initial paint — both tiers run once so the overlay lands at
    // the right pan/zoom + cards land at their model positions.
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () { _syncLayer(); _syncCards(); });
    } else {
      _syncLayer();
      _syncCards();
    }
  }

  function _buildHtmlCard(d) {
    // D37f-rich-card — Validation candidate for the richer Authority
    // card treatment inside the new Authority Cytoscape viewport. The
    // legacy hook class names (`cytoscape-poc-html-card`, `-kind`,
    // `-title`, `-status`) are preserved verbatim for test stability;
    // Authority-specific class names (`authority-html-card-*`) are
    // added alongside via classList.add so the richer structure is
    // explicit in the DOM. This is a validation candidate, not a
    // declaration of final strategic direction.
    var card = document.createElement('article');
    card.className = 'cytoscape-poc-html-card';
    card.classList.add('authority-html-card');
    card.setAttribute('data-node-id', d.id);
    card.setAttribute('data-kind', d.kind || '');
    if (d.isRoot) card.setAttribute('data-root', 'true');

    // Header row — contains the per-kind icon (leading) and the kind
    // chip. The header isolates kind-identity glyphs from the title so
    // the title gets the visual prominence the operator scans for.
    var headerEl = document.createElement('div');
    headerEl.className = 'authority-html-card-header';

    // Per-kind icon — sourced from the MIDAS Icon Registry via the
    // existing `_AUTHORITY_KIND_ICON_KEYS` map. Uses `inlineSvg` (not
    // `cytoscapeDataURI`) so the SVG inherits `currentColor` from CSS
    // and supports a real `<title>` for screen readers. Degrades
    // silently when the registry is unavailable or the key is unknown
    // — the card still renders without an icon. Same safe-fallback
    // contract as `_iconForKind` on the cy-node styling path.
    var iconKey = _AUTHORITY_KIND_ICON_KEYS[d.kind || ''];
    var icons   = window.MIDASExplorerIcons;
    if (iconKey && icons &&
        typeof icons.has       === 'function' && icons.has(iconKey) &&
        typeof icons.inlineSvg === 'function') {
      var iconEl = document.createElement('span');
      iconEl.className = 'authority-html-card-icon';
      iconEl.setAttribute('aria-hidden', 'true');
      iconEl.innerHTML = icons.inlineSvg(iconKey, {
        size:   18,
        title:  _nodeTypeLabel(d.kind || ''),
      });
      headerEl.appendChild(iconEl);
    }

    // Kind chip + title. No invented badges. Real data only.
    var kindEl = document.createElement('span');
    kindEl.className = 'cytoscape-poc-html-card-kind';
    kindEl.classList.add('authority-html-card-kind');
    kindEl.textContent = _nodeTypeLabel(d.kind || '');

    var titleEl = document.createElement('div');
    titleEl.className = 'cytoscape-poc-html-card-title';
    titleEl.classList.add('authority-html-card-title');
    titleEl.textContent = String(d.label || d.id || '');

    headerEl.appendChild(kindEl);
    card.appendChild(headerEl);
    card.appendChild(titleEl);

    // Status row — only emitted if the raw projection actually carries a
    // status field. No placeholder fakery. If the data isn't there, the
    // row is omitted entirely (an honest empty state).
    // D37f-rich-card extends the existing status surface by adding
    // `fail_mode_policy.status` and `escalation_target.status` — both
    // already present in the backend projection when set; no projection
    // change is required.
    var raw = d.raw || {};
    var status =
      (raw.business_service  && raw.business_service.status) ||
      (raw.decision_surface  && raw.decision_surface.status) ||
      (raw.authority_profile && raw.authority_profile.status) ||
      (raw.authority_grant   && raw.authority_grant.status) ||
      (raw.agent             && raw.agent.operational_state) ||
      (raw.fail_mode_policy  && raw.fail_mode_policy.status)  ||
      (raw.escalation_target && raw.escalation_target.status) ||
      '';
    if (status) {
      var statusEl = document.createElement('div');
      statusEl.className = 'cytoscape-poc-html-card-status';
      statusEl.classList.add('authority-html-card-status');
      statusEl.textContent = String(status);
      card.appendChild(statusEl);
    }

    // Meta row — emitted only when at least one meta token is sourced
    // from real projection data. Mirrors the legacy
    // `cytoscape-html-overlay.js` badge pattern. Honest empty state if
    // nothing applies — no invented badges.
    var metaTokens = [];
    if (raw.authority_grant && raw.authority_grant.agent_id) {
      metaTokens.push('AGENT BOUND');
    }
    if (d.isRoot) {
      metaTokens.push('ROOT');
    }
    if (metaTokens.length > 0) {
      var metaEl = document.createElement('div');
      metaEl.className = 'authority-html-card-meta';
      for (var mi = 0; mi < metaTokens.length; mi++) {
        var chipEl = document.createElement('span');
        chipEl.className = 'authority-html-card-meta-chip';
        chipEl.textContent = metaTokens[mi];
        metaEl.appendChild(chipEl);
      }
      card.appendChild(metaEl);
    }

    return card;
  }

  // _updateHtmlCardOverlay — convenience wrapper retained as a
  // public-surface symbol for tests / diagnostics. Runs both tiers
  // (full sync). The continuous-interaction events (pan/zoom vs
  // position) bind to `_syncLayer` / `_syncCards` independently for
  // efficiency.
  function _updateHtmlCardOverlay(cy) {
    void cy; // cy is read via module-scope `_cy`; param kept for backward signature.
    _syncLayer();
    _syncCards();
  }

  function _destroyHtmlCardOverlay() {
    // Cancel both per-tier rAFs so a queued callback can't touch a
    // half-torn overlay.
    if (typeof window.cancelAnimationFrame === 'function') {
      if (_syncLayerRaf) { try { window.cancelAnimationFrame(_syncLayerRaf); } catch (_) {} }
      if (_syncCardsRaf) { try { window.cancelAnimationFrame(_syncCardsRaf); } catch (_) {} }
    }
    _syncLayerRaf = 0;
    _syncCardsRaf = 0;
    if (_cy && _syncLayerBound) {
      try { _cy.off(LAYER_SYNC_EVENTS, _syncLayerBound); } catch (_) {}
    }
    if (_cy && _syncCardsBound) {
      try { _cy.off(CARDS_SYNC_EVENTS, _syncCardsBound); } catch (_) {}
    }
    _syncLayerBound = null;
    _syncCardsBound = null;
    if (_htmlOverlayEl && _htmlOverlayEl.parentNode) {
      _htmlOverlayEl.parentNode.removeChild(_htmlOverlayEl);
    }
    _htmlOverlayEl  = null;
    _htmlCardsByKey = {};
    // D37k-impl-1 — Tear the edge-label overlay down alongside the
    // cards overlay. Both are owned by the renderer's cy mount;
    // both must vanish together so re-renders don't strand orphan
    // DOM.
    _destroyEdgeLabelOverlay();
  }

  // ── D37k-impl-1 — HTML edge-label overlay (single shared chip) ────────
  //
  // The overlay sits ABOVE the html-card overlay (z-index 7 vs cards'
  // z-index 5). The chip is a single shared DOM element re-used
  // across hovers — never per-edge DOM, never iterated. Positioned
  // in model coordinates by `_syncEdgeLabelPosition` and projected
  // through the same layer-tier transform as cards (extended into
  // `_syncLayer`). Hover-only: `_focusEdge` shows it,
  // `_clearInteractionState` hides it.
  //
  // The overlay is installed alongside the cards overlay (see
  // `_installHtmlCardOverlay` lifecycle integration) and destroyed
  // together. No new sync events are bound — the existing
  // `LAYER_SYNC_EVENTS` and `CARDS_SYNC_EVENTS` cover the chip's
  // pan/zoom and drag-tracking needs respectively.

  function _installEdgeLabelOverlay(cy, mount) {
    if (!cy || !mount) return;
    _destroyEdgeLabelOverlay();
    _edgeLabelOverlayEl = document.createElement('div');
    _edgeLabelOverlayEl.className = 'cytoscape-poc-edge-label-overlay';
    _edgeLabelOverlayEl.setAttribute('aria-hidden', 'true');
    _edgeLabelChipEl = document.createElement('div');
    _edgeLabelChipEl.className = 'cytoscape-poc-edge-label-chip';
    _edgeLabelChipEl.setAttribute('hidden', '');
    _edgeLabelChipEl.setAttribute('role', 'presentation');
    _edgeLabelOverlayEl.appendChild(_edgeLabelChipEl);
    mount.appendChild(_edgeLabelOverlayEl);
  }

  function _destroyEdgeLabelOverlay() {
    _edgeLabelFocusedEdgeId = '';
    if (_edgeLabelOverlayEl && _edgeLabelOverlayEl.parentNode) {
      try { _edgeLabelOverlayEl.parentNode.removeChild(_edgeLabelOverlayEl); }
      catch (_) { /* swallow */ }
    }
    _edgeLabelOverlayEl = null;
    _edgeLabelChipEl    = null;
  }

  // _showEdgeLabel populates the chip with the friendly relationship
  // text for the hovered edge and positions it at the edge midpoint.
  // No-op when the overlay is not installed or the edge has no
  // friendly mapping (defensive — `_displayEdgeLabel` falls back to
  // the underscore-replaced kind, so an empty return is rare).
  function _showEdgeLabel(edge) {
    if (!_edgeLabelChipEl || !edge) return;
    var text = _displayEdgeLabel(edge);
    if (typeof text !== 'string' || text === '') {
      _hideEdgeLabel();
      return;
    }
    var id = '';
    try {
      id = typeof edge.id === 'function' ? String(edge.id() || '') :
           (edge.data && typeof edge.data === 'function' ? String(edge.data('id') || '') : '');
    } catch (_) { id = ''; }
    if (!id) { _hideEdgeLabel(); return; }
    _edgeLabelChipEl.textContent = text;
    _edgeLabelFocusedEdgeId = id;
    _syncEdgeLabelPosition();
    _edgeLabelChipEl.removeAttribute('hidden');
  }

  function _hideEdgeLabel() {
    _edgeLabelFocusedEdgeId = '';
    if (!_edgeLabelChipEl) return;
    _edgeLabelChipEl.setAttribute('hidden', '');
    _edgeLabelChipEl.textContent = '';
  }

  // _syncEdgeLabelPosition re-reads the focused edge's midpoint and
  // writes a model-space transform on the chip. Called by:
  //   • `_showEdgeLabel` on initial hover;
  //   • the cards-tier sync (extended at the end of `_syncCards`)
  //     so node-drag-induced midpoint changes track the chip;
  //   • implicitly via the layer-tier transform on pan/zoom.
  // If the focused edge is gone (re-render, exit context view) or
  // has been hidden (authority-context view filter), the chip
  // hides — no stale chip pinned to a dead edge.
  function _syncEdgeLabelPosition() {
    if (!_cy || !_edgeLabelChipEl || !_edgeLabelFocusedEdgeId) return;
    var edge;
    try { edge = _cy.$id(_edgeLabelFocusedEdgeId); } catch (_) { return; }
    if (!edge || !edge.length) { _hideEdgeLabel(); return; }
    if (typeof edge.visible === 'function' && !edge.visible()) {
      _hideEdgeLabel();
      return;
    }
    var mid = null;
    try {
      if (typeof edge.midpoint === 'function') {
        mid = edge.midpoint();
      }
      if (!mid || typeof mid.x !== 'number' || typeof mid.y !== 'number') {
        // Fallback: compute the midpoint from connected node
        // positions. `edge.midpoint()` is the canonical source but
        // we don't assume every Cytoscape build exposes it.
        var src = (typeof edge.source === 'function') ? edge.source() : null;
        var tgt = (typeof edge.target === 'function') ? edge.target() : null;
        if (src && tgt && typeof src.position === 'function' && typeof tgt.position === 'function') {
          var sp = src.position();
          var tp = tgt.position();
          if (sp && tp) {
            mid = { x: (sp.x + tp.x) / 2, y: (sp.y + tp.y) / 2 };
          }
        }
      }
    } catch (_) { return; }
    if (!mid) return;
    var t = 'translate(' + mid.x + 'px,' + mid.y + 'px) translate(-50%, -50%)';
    _edgeLabelChipEl.style.webkitTransform = t;
    _edgeLabelChipEl.style.transform       = t;
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
      // D37k-impl-1 — Canvas-label fallback for the default html-card
      // theme. The primary edge-label surface for D37k is the new
      // HTML edge-label overlay (see `_installEdgeLabelOverlay`),
      // which sits ABOVE the html-card overlay and is therefore
      // visible when an edge passes behind a card. These canvas
      // rules remain as a fallback so the relationship name is still
      // readable if the HTML overlay fails to render (script error,
      // teardown race), and so the cy-on-root-path emphasis chain
      // continues to read on the canvas as today.
      //
      // Mirrors the `authority-thin-card-v1` edge-label styling:
      // friendly per-kind text via `_displayEdgeLabel`, 12 px,
      // weight 500, full-contrast `--on-surface` colour, round-
      // rectangle chip with 5 px padding, soft outline border.
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
          'font-weight':                      '500',
          'color':                            pal.onSurface,
          'text-background-color':            pal.surface,
          'text-background-opacity':          0.95,
          'text-background-shape':            'round-rectangle',
          'text-background-corner-radius':    3,
          'text-background-padding':          '5px',
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
    // D37k-impl-1 — Hover-only chip; every interaction-state reset
    // (mouseout-with-no-selection, background tap, _focusNode taking
    // over after a node hover) hides the relationship chip. There
    // is no "pin" gesture in D37k — losing the hover loses the
    // chip.
    _hideEdgeLabel();
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
    // D37k-impl-1 — Surface the friendly relationship label in the
    // HTML overlay chip. The canvas-drawn label (from the html-card
    // theme rule added in this tranche) remains as a fallback in
    // case the overlay fails to render. `_clearInteractionState`
    // above already hid any prior chip — `_showEdgeLabel` then
    // re-shows for the new edge.
    _showEdgeLabel(edge);
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
      // D33x-list-mode — Floating PoC inspector card retired. The
      // production right drawer (`#gmap-details`) is the canonical
      // selected-node surface, fed via the carrier DOM contract
      // (`_renderInspectorCarriers` paints hidden `.gmap-node`
      // elements under `#gmap-canvas` with `data-node-details`
      // JSON) plus the renderer hook below.
      //
      // The PoC's mapped node id (`kind+':'+id`) matches the
      // production `_refKey({kind,id})` format unchanged. The
      // renderer hook lens-dispatches into
      // `authorityInspector.selectNode(nodeId)`, which then reads
      // the carrier's `data-node-details` and pushes fields through
      // the lens-agnostic inspector frame into the right drawer.
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
        // D33x-list-mode — Floating PoC inspector card retired.
        // Background tap no longer needs to clear the card; the
        // right drawer's own empty/cleared state is driven by the
        // production selection-routing path.
      }
    });

    // D37h — Double-click a node = camera-focus on that node.
    // Cytoscape `dbltap` is a normalised double-tap event (two
    // subsequent `click`s or two subsequent `touchstart`s); the
    // existing single-tap select handler above continues to fire
    // first, so dbltap arrives after the node is already selected.
    //
    // This is a CAMERA operation only — it does not change the
    // graph contents, filter the projection, or re-query the
    // backend. HTML cards stay pointer-passive; the event target
    // is the underlying Cytoscape node, which is what dbltap
    // delivers.
    _cy.on('dbltap', 'node', function (evt) {
      var node = evt && evt.target;
      if (!node) return;
      _zoomToNode(_cy, node);
    });

    // D37j — Auto-exit authority-context view when the operator
    // selects a different node. Bound to `select` only (not
    // `unselect`): an empty selection is a valid state inside
    // context view (the operator may de-select to read the context
    // without exiting). Re-selecting the focal node is a no-op.
    _cy.on('select', 'node', function () {
      _checkAutoExitContext();
    });

    // D37h — Bind external viewport-change and selection-change
    // handlers to this fresh cy instance. Subscribers attach via
    // `cytoscapePoc.onViewportChanged` / `onSelectionChanged`; the
    // registries survive cy teardown so re-mounts re-attach the
    // same subscribers (without the subscriber having to know).
    _attachExternalHandlersToCy(_cy);
  }

  // ── Render lifecycle ─────────────────────────────────────────────────

  function _renderPayload(payload) {
    var mount = _ensureMount();
    // D35f — `_ensureMount` returns null when the GraphViewport
    // host is unavailable (the legacy fallback that previously
    // inserted into `.governance-map-canvas-scroll` was retired).
    // Fail safely: skip the render rather than building a parallel
    // mount surface.
    if (!mount) return;
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

    // D33a-spike-2 + D37f — Install the Authority HTML-card overlay
    // unconditionally. Pre-D37f this was gated on
    // `_activeTheme === 'html-card'`; D37f retired the gate so HTML
    // cards are the production Authority visual regardless of the
    // underlying cy theme. The overlay is purely presentational
    // (pointer-events:none); Cytoscape continues to own drag, hover,
    // select, pan, and zoom. _destroyHtmlCardOverlay is wired into
    // _destroyCy and _uninstallPoc so service refresh / lens unmount
    // leave no DOM.
    _installHtmlCardOverlay(_cy, mount, elements);
    // D37k-impl-1 — Install the edge-label overlay AFTER the cards
    // overlay so DOM order matches z-index intent (chip layer sits
    // above the cards layer). The overlay is owned by the same
    // mount and torn down by `_destroyHtmlCardOverlay` →
    // `_destroyEdgeLabelOverlay` (see lifecycle integration).
    _installEdgeLabelOverlay(_cy, mount);
    // D34a-cytoscape-html-overlay-spike — When the new spike gate
    // `?htmlCards=1` is set alongside `?cytoscape=1`, install the
    // MIDAS-rich HTML overlay. Distinct from the `?cyTheme=html-card`
    // theme above (which renders thin cards). The spike's overlay
    // is self-gated via `cytoscapeHtmlOverlay.isActive()` so the
    // call is a no-op when the flag is absent. Teardown is hooked
    // into `_destroyCy` below.
    var _htmlOverlay = window.MIDASExplorerGraph && window.MIDASExplorerGraph.cytoscapeHtmlOverlay;
    if (_htmlOverlay && typeof _htmlOverlay.install === 'function') {
      _htmlOverlay.install(_cy, { mount: mount, elements: elements });
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
    //
    // D33x-fit-zoom-root — _settleFit delegates to the asymmetric
    // `_fitToAvailableCanvas` helper. The original symmetric
    // `_cy.fit(undefined, _safeAreaPadding())` budget was uniform on
    // all four sides, which forced the largest overlay's width
    // (e.g. expanded legend = 260 + buffer = 276 px) onto every
    // side, leaving the graph with only ~half the viewport. The new
    // helper uses per-side insets (legend on left only, inspector +
    // production drawer on right only, camera cluster bottom-right
    // only) via `cy.viewport({zoom, pan})`, so labels are
    // materially larger and the graph uses the full vertical
    // height. `_safeAreaPadding` remains exported so tests +
    // headless paths that still want a uniform value can reach it.
    function _settleFit() {
      if (!_cy) return;
      try {
        _cy.resize();
        _fitToAvailableCanvas(_cy);
      } catch (_) { /* swallow */ }
    }
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () {
        _settleFit();
        window.requestAnimationFrame(_settleFit);
      });
    }
    setTimeout(_settleFit, 120);

    // D37b — Diagnostics-panel / Surface-posture-panel / Workbench bridge.
    //
    // Pre-D37b the production Authority view (authority-graph-view.js)
    // called `authorityDiagnosticsPanel.render(payload)`,
    // `authoritySurfacePosturePanel.render(payload)`, and
    // `authorityWorkbench.render()` after every successful paint. The
    // Cytoscape PoC's `_pocRefresh` routed around the native view, so
    // those panels never re-rendered when Cytoscape was active.
    //
    // The cached `_lastAuthorityProjection` set above lets the
    // workbench module (which reads from the cache) work, but the
    // diagnostics + posture panels need to be called explicitly with
    // the payload. Each call is wrapped defensively so a module
    // absence (test isolation, early boot, future refactor) cannot
    // break the Cytoscape render path.
    //
    // The integration is intentionally narrow: it preserves the
    // EXISTING panel modules without duplicating their logic.
    try {
      var diagPanel = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityDiagnosticsPanel;
      if (diagPanel && typeof diagPanel.render === 'function') diagPanel.render(payload);
    } catch (_) { /* swallow */ }
    try {
      var posturePanel = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authoritySurfacePosturePanel;
      if (posturePanel && typeof posturePanel.render === 'function') posturePanel.render(payload);
    } catch (_) { /* swallow */ }
    try {
      var workbenchMod = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityWorkbench;
      if (workbenchMod && typeof workbenchMod.render === 'function') workbenchMod.render();
    } catch (_) { /* swallow */ }

    // D33a-impl-1 — No longer hijack #gmap-status. The PoC indicator
    // moved into the legend chip; the production status pill stays
    // available for the existing Authority workbench.
  }

  function _renderUnavailable(mount, message) {
    // D37d-authority-cytoscape-mount-visibility-fix — Defensive null
    // guard. `_ensureMount()` returns `null` when the GraphViewport
    // host is unavailable (documented at L1452-1463); the pre-D37d
    // code path then called `mount.appendChild(...)` and threw a
    // `TypeError`, silently killing the render path. This guard makes
    // the function a safe no-op in that case so the render path
    // surfaces a clean "no mount" diagnostic upstream instead of an
    // uncaught exception. See D37c assessment §10 Candidate #2.
    if (!mount) return;
    _destroyCy();
    // D33x-list-mode    — Floating PoC inspector aside retired.
    // D33x-left-poc-panel — Floating legend aside retired. The
    //                     unavailable overlay now lands on a bare
    //                     mount with no PoC chrome to re-attach.
    // Remove any prior unavailable overlay before appending — repeated
    // refreshes must not accumulate divs.
    _clearOverlays(mount);
    var overlay = document.createElement('div');
    overlay.className = 'cytoscape-poc-unavailable';
    overlay.textContent = message;
    mount.appendChild(overlay);
  }

  // ── Lens dispatch override (RETIRED in D37p-clean-1) ─────────────────
  //
  // The pre-D37p-clean-1 module wired a `lensImpl` and a deferred
  // `_registerWhenReady` IIFE against the dead
  // `MIDASExplorerGraph.renderer.register('authority', …)` dispatcher.
  // Diagnostic finding (recorded just below): the dispatcher had zero
  // call-sites at runtime, so neither the lensImpl's `render` nor its
  // `clear` ever fired. Both are removed here. The live Authority
  // dispatch flows are unchanged:
  //
  //   • GraphViewport registration — `viewport.register('authority',
  //     _authorityRendererFactory)` (D35g, see the bottom of this
  //     file) remains the host-level activation seam.
  //   • Live refresh — `authorityView.refresh` is patched below to
  //     route through `_pocRefresh`, which is the production
  //     Authority data path.
  //   • Public surface — `cytoscapePoc.{getCy, fit, zoomBy, …}` and
  //     the bus / bridge / pane delegates that depend on it all
  //     remain.
  //
  // `_uninstallPoc` is preserved (still exported as `_uninstall` for
  // tests and manual diagnostics, and called by `_uninstallPoc`'s
  // own routing through the GraphViewport host's deactivate path
  // — see L1976-2030).

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
    _destroy:                 _destroyCy,
    // D33a-impl-1 — full teardown for tests and manual diagnostics.
    _uninstall:               _uninstallPoc,
    _safeAreaPadding:         _safeAreaPadding,
    // D33x-fit-zoom-root — public surface used by the toolbar
    // bridge (`authority-cytoscape-toolbar.js`) to drive +/-, fit,
    // and centre-on-root from the existing MIDAS camera cluster.
    // `getCy()` returns null when no PoC graph is mounted; every
    // helper is a no-op on null inputs, so the toolbar bridge can
    // call them unconditionally.
    getCy:                    function () { return _cy; },
    fit:                      function (opts) { _fitToAvailableCanvas(_cy, opts); },
    zoomBy:                   function (factor) { _zoomBy(_cy, factor); },
    centerOnRoot:             function () { _centerOnRoot(_cy); },
    findRootNode:             function () { return _findRootNode(_cy); },
    _fitToAvailableCanvas:    _fitToAvailableCanvas,
    _zoomBy:                  _zoomBy,
    _centerOnRoot:            _centerOnRoot,
    _findRootNode:            _findRootNode,
    ZOOM_STEP_FACTOR:         ZOOM_STEP_FACTOR,
    // D37h — Camera/navigation surface for the toolbar bridge.
    // Every helper is camera-only; none mutate the graph contents,
    // filter the projection, or re-query the backend.
    isReady:                  function () { return !!_cy; },
    zoomToSelected:           function () { _zoomToSelected(_cy); },
    zoomToNode:               function (nodeId) {
      if (!_cy || !nodeId || typeof _cy.$id !== 'function') return;
      var node;
      try { node = _cy.$id(String(nodeId)); } catch (_) { return; }
      if (!node || (typeof node.length === 'number' && node.length === 0)) return;
      _zoomToNode(_cy, node);
    },
    resetView:                function () { _resetView(_cy); },
    getZoomPercent:           function () { return _getZoomPercent(_cy); },
    onViewportChanged:        _onViewportChanged,
    onSelectionChanged:       _onSelectionChanged,
    _zoomToSelected:          _zoomToSelected,
    _zoomToNode:              _zoomToNode,
    _resetView:               _resetView,
    _getZoomPercent:          _getZoomPercent,
    // D37j — Client-side authority-context view (projection-style
    // filter; not camera). Operator clicks `View authority context`
    // in the toolbar; the renderer hides every cy element outside
    // the directed-traversal authority context of the focal node
    // (predecessors ∪ successors ∪ self + BS-default policy edges).
    // No backend fetch; no re-rooting; no element removal.
    viewAuthorityContext:        function () { return _viewAuthorityContext(); },
    exitAuthorityContext:        function () { return _exitAuthorityContext(); },
    toggleAuthorityContext:      function () { return _toggleAuthorityContext(); },
    isAuthorityContextActive:    function () { return _isAuthorityContextActive(); },
    canViewAuthorityContext:     function () { return _canViewAuthorityContext(); },
    onAuthorityContextChanged:   _onAuthorityContextChanged,
    _AUTHORITY_CONTEXT_ELIGIBLE_KINDS: _AUTHORITY_CONTEXT_ELIGIBLE_KINDS,
    _computeAuthorityContext:    _computeAuthorityContext,
    // D33x-list-mode — Cytoscape-backed list mode public surface.
    // Consumed by the lens-aware branch in `index.html`'s
    // `setWorkbenchMode('form')` handler when the active lens is
    // `authority` and `body.cytoscape-poc-active` is set.
    setViewMode:              setViewMode,
    getViewMode:              getViewMode,
    applyListLayout:          applyListLayout,
    applyGraphLayout:         applyGraphLayout,
    _computeListPositions:    _computeListPositions,
    LIST_GROUP_ORDER:         LIST_GROUP_ORDER.slice(),
    LIST_MAX_COLUMNS:         LIST_MAX_COLUMNS,
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
    // D37k-impl-1 — Edge-label overlay diagnostic surface.
    _installEdgeLabelOverlay:  _installEdgeLabelOverlay,
    _destroyEdgeLabelOverlay:  _destroyEdgeLabelOverlay,
    _showEdgeLabel:            _showEdgeLabel,
    _hideEdgeLabel:            _hideEdgeLabel,
    _syncEdgeLabelPosition:    _syncEdgeLabelPosition,
    // D35d-port-authority-cytoscape-to-graphviewport (D37b promoted) —
    // Renderer factory + internal teardown helper exposed for tests
    // and host-driven activation paths. As of D37b the factory is
    // registered with the GraphViewport host under the PRODUCTION
    // renderer id `'authority'` at module init, and `_ensureMount`
    // activates by id via `viewport.activateById('authority')` rather
    // than passing the factory inline.
    _rendererFactory:          _authorityRendererFactory,
    _teardownPocResources:     _teardownPocResources,
  };

  // D35g-graphviewport-renderer-registry (D37b promoted) — register
  // the Authority renderer factory with the GraphViewport host so
  // normal Authority lens activation can reach Cytoscape via
  // `viewport.activateById('authority')`, and so external callers
  // (toolbar, lens orchestration, tests) can discover the renderer
  // via `viewport.listRegistered()`/`hasRenderer('authority')`.
  // Wrapped defensively because module load must never break the
  // page if the host script failed to load or exposes an unexpected
  // shape.
  (function _registerWithGraphViewport() {
    try {
      var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
      if (vp && typeof vp.register === 'function') {
        vp.register('authority', _authorityRendererFactory);
      }
    } catch (_) { /* swallow — must not break page load */ }
  })();

  // D37p-impl-4 — Authority camera bus delegate registration.
  //
  // Wraps the existing public camera methods on the cytoscapePoc
  // surface (`zoomBy`, `fit`, `centerOnRoot`, `zoomToSelected`,
  // `resetView`, `getZoomPercent`) in the locked bus command
  // vocabulary. The shared graphCameraToolbarAdapter dispatches
  // through graphCameraBus, which routes Authority commands here
  // when the active renderer is `'authority'`. This retires the
  // per-lens capture-phase camera intercept in
  // authority-cytoscape-toolbar.js for the six camera-cluster
  // buttons; non-camera Authority controls (focus-toggle,
  // authority-context view) continue to use that file.
  //
  // Defensive: registration is wrapped in try/catch so a missing or
  // malformed bus must not break the Authority module's own load.
  (function _registerAuthorityCameraBusDelegate() {
    try {
      var g   = window.MIDASExplorerGraph;
      var bus = g && g.graphCameraBus;
      var poc = g && g.cytoscapePoc;
      if (!bus || typeof bus.registerLens !== 'function') return;
      if (!poc) return;
      var step = (typeof poc.ZOOM_STEP_FACTOR === 'number' && poc.ZOOM_STEP_FACTOR > 0)
        ? poc.ZOOM_STEP_FACTOR
        : 1.2;
      bus.registerLens('authority', {
        zoomIn:        function () { if (typeof poc.zoomBy === 'function') poc.zoomBy(step); },
        zoomOut:       function () { if (typeof poc.zoomBy === 'function') poc.zoomBy(1 / step); },
        fit:           function () { if (typeof poc.fit === 'function') poc.fit(); },
        reset:         function () { if (typeof poc.resetView === 'function') poc.resetView(); },
        focusRoot:     function () { if (typeof poc.centerOnRoot === 'function') poc.centerOnRoot(); },
        focusSelected: function () { if (typeof poc.zoomToSelected === 'function') poc.zoomToSelected(); },
        // D37q-viewport-4-impl — canonical camera-bus `getZoom()` unit
        // is RATIO (1.0 = 100%). The Authority engine's display helper
        // `cytoscapePoc.getZoomPercent()` still returns an integer
        // percent for the toolbar zoom-percent badge; the bus delegate
        // converts to ratio so cross-lens consumers see consistent
        // units across `native-context`, `context`, and `authority`.
        getZoom: function () {
          if (typeof poc.getZoomPercent !== 'function') return null;
          var pct;
          try { pct = poc.getZoomPercent(); }
          catch (_) { return null; }
          if (typeof pct !== 'number' || !isFinite(pct) || pct <= 0) return null;
          return pct / 100;
        },
      });
    } catch (_) { /* swallow — must not break page load */ }
  })();

  // D37p-authority-1-impl — Authority Selection Bridge Delegate.
  //
  // Publishes Cytoscape selection state into the shared
  // `graphSelectionBridge` so cross-lens consumers can observe
  // Authority selection through the locked event vocabulary
  // (`selection_changed` / `selection_cleared`). Authority's
  // existing local subscribers (canvas-edge tabs, toolbar zoom-
  // selected button enablement, authority-context eligibility)
  // stay authoritative for engine-coupled behaviour; the bridge
  // push is purely additive.
  //
  // Delegate methods registered with the bridge are READ / ACTION
  // only (`getCurrentCard`, `getCurrentNodeRef`, `handleAction`).
  // There is no `delegate.selectCard` — the recursion-discipline
  // contract at graph-selection-bridge.js requires that external
  // `bridge.selectCard(payload)` callers cannot drive a Cytoscape
  // selection event through the Authority delegate. The one-way
  // flow this tranche establishes is:
  //
  //   Cytoscape select/unselect event
  //     → existing Authority subscribers (unchanged)
  //     → graphSelectionBridge.selectCard / clearSelection
  //     → platform subscribers (pane shell, future consumers)
  //
  // Defensive: every reach into the bridge / `_cy` is wrapped so a
  // missing or malformed peer cannot break module load. The push
  // helper is registered through the existing `_onSelectionChanged`
  // registry so it survives cy teardown / re-mount without manual
  // re-binding — `_attachExternalHandlersToCy` already does that
  // job for every entry in the registry.
  (function _registerAuthoritySelectionBridgeDelegate() {
    function _bridge() {
      var g = window.MIDASExplorerGraph;
      return (g && g.graphSelectionBridge) || null;
    }

    function _selectedCyNode() {
      if (!_cy || typeof _cy.elements !== 'function') return null;
      var sel;
      try { sel = _cy.elements(':selected'); } catch (_) { return null; }
      if (!sel || typeof sel.length !== 'number' || sel.length !== 1) return null;
      var n = sel[0];
      if (!n || typeof n.data !== 'function') return null;
      return n;
    }

    function _readCarrierDetails(kind, id) {
      try {
        if (typeof document === 'undefined' || !document.querySelector) return null;
        var key = String(kind) + ':' + String(id);
        var el = document.querySelector('[data-node-id="' + key + '"][data-node-details]');
        if (!el) return null;
        var raw = el.getAttribute('data-node-details');
        if (!raw) return null;
        return JSON.parse(raw);
      } catch (_) { return null; }
    }

    function _materialiseAuthoritySelection(node) {
      if (!node || typeof node.data !== 'function') return null;
      var id    = '';
      var kind  = '';
      var label = '';
      var name  = '';
      try {
        id    = String(node.data('id')    || '');
        kind  = String(node.data('kind')  || '');
        label = String(node.data('label') || '');
        name  = String(node.data('name')  || '');
      } catch (_) { /* swallow */ }
      if (!id) return null;
      var sourceNodeRef = { kind: kind, id: id };
      var card = {
        id:            id,
        kind:          kind,
        label:         label,
        name:          name || label,
        sourceNodeRef: sourceNodeRef,
      };
      var details = _readCarrierDetails(kind, id);
      if (details && typeof details === 'object') {
        card.details = details;
      }
      return { id: id, kind: kind, sourceNodeRef: sourceNodeRef, card: card };
    }

    function getCurrentCard() {
      var sel = _materialiseAuthoritySelection(_selectedCyNode());
      return sel ? sel.card : null;
    }

    function getCurrentNodeRef() {
      var sel = _materialiseAuthoritySelection(_selectedCyNode());
      return sel ? sel.sourceNodeRef : null;
    }

    // handleAction is intentionally narrow in this tranche. Returns
    // null for unsupported actions; never throws. Future tranches
    // may route Authority-specific actions (open-record, reframe,
    // open-workbench-tab) through this hook — out of scope here.
    function handleAction(action) {
      if (!action || typeof action !== 'object') return null;
      void action;
      return null;
    }

    function _maybeAssertActiveLens(bridge) {
      if (!bridge || typeof bridge.setActiveLens !== 'function') return;
      try {
        var g = window.MIDASExplorerGraph;
        var vp = g && g.viewport;
        if (vp && typeof vp.getActiveRendererId === 'function') {
          var activeId = vp.getActiveRendererId();
          if (activeId === 'authority') bridge.setActiveLens('authority');
        }
      } catch (_) { /* swallow */ }
    }

    function _publishAuthoritySelectionToSharedBridge() {
      var bridge = _bridge();
      if (!bridge) return;
      _maybeAssertActiveLens(bridge);
      var node = _selectedCyNode();
      if (!node) {
        try { bridge.clearSelection(); } catch (_) { /* swallow */ }
        return;
      }
      var sel = _materialiseAuthoritySelection(node);
      if (!sel) {
        try { bridge.clearSelection(); } catch (_) { /* swallow */ }
        return;
      }
      try {
        bridge.selectCard({
          lens:          'authority',
          id:            sel.id,
          kind:          sel.kind,
          sourceNodeRef: sel.sourceNodeRef,
          card:          sel.card,
          meta: {
            source:     'authority-cytoscape',
            selectedAt: Date.now(),
          },
        });
      } catch (_) { /* swallow */ }
    }

    function _registerDelegate() {
      var bridge = _bridge();
      if (!bridge || typeof bridge.registerLens !== 'function') return false;
      try {
        bridge.registerLens('authority', {
          getCurrentCard:    getCurrentCard,
          getCurrentNodeRef: getCurrentNodeRef,
          handleAction:      handleAction,
        });
        _maybeAssertActiveLens(bridge);
        return true;
      } catch (_) { return false; }
    }

    // Register the delegate at module init. Idempotent — the
    // bridge's REPLACE policy makes a duplicate call safe.
    _registerDelegate();

    // Hook the publish helper into the existing selection-change
    // registry. `_onSelectionChanged` records the handler in
    // `_selectionChangeHandlers`; `_attachExternalHandlersToCy`
    // attaches every registered handler to the live `_cy` on each
    // fresh mount, so a single registration here survives every
    // cy teardown / re-mount without re-binding.
    try {
      if (typeof _onSelectionChanged === 'function') {
        _onSelectionChanged(_publishAuthoritySelectionToSharedBridge);
      }
    } catch (_) { /* swallow */ }
  })();
})();
