// /explorer/assets/js/graph/authority/cytoscape-html-overlay.js
//
// D34a-cytoscape-html-overlay-spike — MIDAS-owned HTML overlay layer.
//
// ── D37r-tranche-B' migration note ────────────────────────────────
//
// As of tranche B' this file is Authority's TEMPLATE module only.
// The position-sync mechanism (rAF-coalesced render/pan/zoom/
// position/layoutstop subscription, per-card translate3d writes
// from `node.renderedPosition()`, ResizeObserver, window resize
// fallback, listener cleanup) is owned by the shared platform
// module:
//
//   /explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js
//
// Authority's `install(cy, options)` now calls
// `graphCytoscapeOverlay.mount(cy, mount, { lensId: 'authority',
// template: { create(node) → DOM }, keyForNode, pointerEvents: 'auto',
// layerClassName: OVERLAY_CLASS })` and stores the returned handle.
// `_buildCard`, `_wireCardClick`, and the per-kind helpers below
// remain in this file because they are Authority-specific (projection
// shape, click routing into `_rendererHooks.selectNode`,
// kind-vocabulary).
//
// Strategic rule (see graph-cytoscape-overlay.js for the canonical
// statement): no graph lens may implement its own HTML overlay
// mechanics; it may only supply a template to the shared module.
// This file does NOT subscribe to cy viewport events, does NOT
// iterate `cy.nodes()` for `renderedPosition()`, and does NOT
// create a position-sync rAF loop. All of those concerns live in
// the shared platform module.
//
// Goal: prove that Cytoscape can drive graph topology (layout, pan,
// zoom, fit, selection) while MIDAS renders rich HTML cards over the
// nodes — cards that look materially closer to the existing Context
// Graph cards than the thin Authority Cytoscape card.
//
// Inspiration: the the node-html-label extension extension. Conceptual
// pattern only. NO source code copied. NO third-party dependency
// installed, vendored, or imported. Pure local code.
//
// Activation gate (both query params must be present):
//   ?cytoscape=1   — the existing Authority Cytoscape PoC gate
//   ?htmlCards=1   — this spike's gate
//
// The module is loaded unconditionally by index.html so the rest of
// the Explorer can read its `isActive()` flag, but `install()` is a
// no-op unless both gate params are set. There is no production-
// visible toggle.
//
// Public surface (window.MIDASExplorerGraph.cytoscapeHtmlOverlay):
//   isActive()                        — true when both gate flags set
//   install(cy, options)              — mount overlay layer + render cards
//                                       options.mount     — DOM parent for the layer
//                                       options.elements  — { nodes, edges } as
//                                                           returned by the PoC mapper
//   destroy()                         — remove DOM + event handlers
//   refresh(cy, elements)             — re-render after a graph reload
//
// Internal helpers `_escHtml`, `_buildCard`, `_sync`, and the active
// state are also exposed for tests.
//
// Card visual baseline (mirrors the Context Graph gmap-node markup at
// `graph-renderer.js:addNode`):
//
//   <button class="midas-cy-overlay-card" data-node-id="..." data-kind="...">
//     <span class="gmap-node-label">EYEBROW</span>     <!-- inert sub-class -->
//     <span class="gmap-node-name">Primary Name</span>
//     <div class="gmap-node-meta">
//       <span>status</span>
//       <span class="gmap-badge bind">AI BOUND</span>
//     </div>
//   </button>
//
// The `.gmap-node-*` sub-classes are inert when used outside the
// production `.gmap-node` selector — they only carry typography +
// colour. Using them here means free visual parity with the Context
// Graph cards without copying or duplicating CSS.
//
// Selection routing:
//   Clicking a card calls `_rendererHooks.selectNode(nodeId)` so the
//   MIDAS production right drawer updates exactly as it does for a
//   native Cytoscape tap. The cy node is also selected so any
//   downstream cy event handlers see the same state.
//
// Lifecycle:
//   install()  → create overlay container, build cards, attach event
//                listeners, dim native cy nodes to opacity 0 (the
//                HTML overlay becomes the visible layer; the native
//                node still hit-tests so drag handlers work if a
//                future iteration needs them).
//   destroy()  → cancel rAF, detach cy + window listeners + the
//                ResizeObserver, remove DOM, restore native cy node
//                opacity.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var OVERLAY_CLASS = 'midas-cy-overlay-layer';
  var CARD_CLASS    = 'midas-cy-overlay-card';
  var SYNC_EVENTS   = 'render pan zoom position layoutstop';

  // Module state. Single overlay instance at a time matches the
  // PoC's single-cy-instance model; install() destroys any prior
  // overlay first.
  //
  // D37r-tranche-B' — the position-sync / listener / ResizeObserver
  // / window-resize machinery is now owned by the shared
  // `graphCytoscapeOverlay` platform module. This file keeps only:
  //   • `_handle` — the handle returned by the shared module's
  //     `mount(...)`. `destroy()` and `refresh()` delegate to it.
  //   • `_cy` / `_mount` — references retained so `refresh(...)`
  //     can re-mount on the same surface without the caller
  //     re-supplying it.
  //   • `_elementsByKey` — Authority's element-data lookup used by
  //     the template's `create(node)` to resolve a node id back to
  //     the typed-data block produced by the PoC's mapper.
  //   • `_dimmedNativeNodes` — the cy-node opacity-dim flag, an
  //     Authority-specific visual decision (overlay layer is the
  //     visible card; cy node is hidden).
  var _cy             = null;
  var _mount          = null;
  var _handle         = null;
  var _elementsByKey  = null;   // { [cyNodeId]: entry.data }
  var _dimmedNativeNodes = false;

  // _isActive reads the URL each call so the gate works under
  // history.replaceState navigations + test isolation.
  function _isActive() {
    try {
      var sp = new URLSearchParams(window.location.search);
      return sp.get('cytoscape') === '1' && sp.get('htmlCards') === '1';
    } catch (_) {
      return false;
    }
  }

  // _escHtml is the local escape helper. textContent is preferred
  // for every user/projection-supplied string; this helper only
  // exists for the small number of attribute interpolations below
  // (`data-kind="<kind>"`) where textContent cannot apply. Matches
  // the existing MIDASExplorerUtils.escHtml semantics so behaviour
  // is consistent across the Explorer.
  function _escHtml(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  // _nodeTypeLabel returns the eyebrow string for a card. Mirrors
  // the labels Context Graph uses (`CAPABILITY`, `DECISION SURFACE`,
  // `PROCESS`) for kinds that overlap, and falls back to a
  // human-readable version of the projection kind for Authority-
  // specific kinds.
  function _nodeTypeLabel(kind) {
    var k = String(kind || '');
    switch (k) {
      case 'business_service':  return 'BUSINESS SERVICE';
      case 'decision_surface':  return 'DECISION SURFACE';
      case 'authority_profile': return 'AUTHORITY PROFILE';
      case 'authority_grant':   return 'AUTHORITY GRANT';
      case 'agent':             return 'AGENT';
      case 'fail_mode_policy':  return 'FAIL-MODE POLICY';
      case 'escalation_target': return 'ESCALATION TARGET';
      case 'capability':        return 'CAPABILITY';
      case 'process':           return 'PROCESS';
      case 'ai_system':         return 'AI SYSTEM';
      default:                  return k.replace(/_/g, ' ').toUpperCase();
    }
  }

  // _statusFor returns the secondary line for a card. Pulls from
  // the projection's typed-data block first, then from common
  // top-level fields. Returns '' when nothing is available — the
  // status row is omitted entirely in that case (honest empty).
  function _statusFor(data) {
    if (!data) return '';
    var raw = data.raw || {};
    return (
      (raw.business_service  && raw.business_service.status) ||
      (raw.decision_surface  && raw.decision_surface.status) ||
      (raw.authority_profile && raw.authority_profile.status) ||
      (raw.authority_grant   && raw.authority_grant.status) ||
      (raw.agent             && raw.agent.operational_state) ||
      (raw.fail_mode_policy  && raw.fail_mode_policy.status) ||
      data.status || ''
    );
  }

  // _badgesFor returns an array of `{ cls, text }` badges for a
  // card. Mirrors the Context Graph badge pattern (AI BOUND / NO AI /
  // GAP). For Authority kinds where the projection doesn't surface
  // these specific concepts, the array is empty — no invented data.
  function _badgesFor(data) {
    if (!data) return [];
    var raw = data.raw || {};
    var out = [];
    // Decision surfaces in the Authority projection don't carry
    // `ai_bindings`; the cytoscape PoC's projection is governance-
    // shaped, not context-shaped. We only emit badges that the
    // underlying typed-data actually exposes.
    if (raw.authority_grant && raw.authority_grant.agent_id) {
      out.push({ cls: 'bind', text: 'AGENT BOUND' });
    }
    if (data.isRoot) {
      out.push({ cls: 'bind', text: 'ROOT' });
    }
    return out;
  }

  // _buildCard creates the DOM for a single card. Uses textContent
  // for every user/projection-supplied string; the small static
  // markup template is local and contains no interpolation.
  // Exposed for tests.
  function _buildCard(data) {
    var card = document.createElement('button');
    card.type = 'button';
    card.className = CARD_CLASS;
    if (data && data.kind) {
      card.classList.add('midas-cy-overlay-card-' + _cssToken(data.kind));
    }
    if (data && data.isRoot) card.setAttribute('data-root', 'true');
    card.setAttribute('data-node-id', String((data && data.id) || ''));
    card.setAttribute('data-kind',    String((data && data.kind) || ''));
    card.setAttribute(
      'aria-label',
      _nodeTypeLabel(data && data.kind) + ': ' + String((data && (data.label || data.name)) || (data && data.id) || '')
    );

    var eyebrow = document.createElement('span');
    eyebrow.className = 'gmap-node-label';
    eyebrow.textContent = _nodeTypeLabel(data && data.kind);
    card.appendChild(eyebrow);

    var name = document.createElement('span');
    name.className = 'gmap-node-name';
    name.textContent = String((data && (data.label || data.name)) || (data && data.id) || '');
    card.appendChild(name);

    var status  = _statusFor(data);
    var badges  = _badgesFor(data);
    if (status || badges.length > 0) {
      var meta = document.createElement('div');
      meta.className = 'gmap-node-meta';
      if (status) {
        var statusEl = document.createElement('span');
        statusEl.textContent = String(status);
        meta.appendChild(statusEl);
      }
      for (var i = 0; i < badges.length; i++) {
        var b = badges[i];
        var bEl = document.createElement('span');
        bEl.className = 'gmap-badge ' + _cssToken(b.cls || '');
        bEl.textContent = String(b.text || '');
        meta.appendChild(bEl);
      }
      card.appendChild(meta);
    }

    return card;
  }

  // _cssToken strips any character that isn't safe inside a CSS
  // class name. Defensive against future kind strings containing
  // punctuation or whitespace.
  function _cssToken(s) {
    return String(s || '').replace(/[^a-zA-Z0-9_-]/g, '');
  }

  // _hooks returns the renderer-hook registry used by the PoC's
  // tap handler. Same lens-aware dispatch path; we just trigger it
  // from a card click instead of a native cy tap.
  function _hooks() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || null;
  }

  // _wireCardClick attaches the click handler that selects the
  // underlying cy node + dispatches into the production right
  // drawer. `stopPropagation` prevents the click from bubbling up
  // to the overlay layer (which has `pointer-events: none` anyway,
  // so this is belt-and-braces).
  function _wireCardClick(card, nodeId) {
    card.addEventListener('click', function (ev) {
      if (ev) ev.stopPropagation();
      if (!_cy) return;
      try {
        var n = _cy.$id(nodeId);
        if (n && n.length) {
          _cy.elements().unselect();
          n.select();
        }
      } catch (_) { /* swallow */ }
      var h = _hooks();
      if (h && typeof h.selectNode === 'function') {
        try { h.selectNode(nodeId); } catch (_) { /* swallow */ }
      }
    });
    // Keyboard activation — `<button>` already responds to Enter /
    // Space, but we also forward to the same selection path so a
    // keyboard click triggers the right drawer.
    card.addEventListener('keydown', function (ev) {
      if (ev && (ev.key === 'Enter' || ev.key === ' ')) {
        ev.preventDefault();
        card.click();
      }
    });
  }

  // _sync — RETIRED in D37r-tranche-B'.
  //
  // The per-card position-sync mechanism (read `node.renderedPosition()`,
  // write `transform: translate3d(...)` on each overlay card, rAF-
  // coalesce over `cy.on('render pan zoom position layoutstop', ...)`,
  // attach ResizeObserver on the cy mount, attach window resize
  // fallback) was moved out of this file and into the shared platform
  // module at /explorer/assets/js/graph/graph-platform/graph-cytoscape-
  // overlay.js. This file now retains ONLY Authority's template
  // (`_buildCard`, `_nodeTypeLabel`, `_statusFor`, `_badgesFor`,
  // `_cssToken`) and Authority's per-card click-routing (`_wireCardClick`).
  // Position sync, listener wiring, and teardown are owned by
  // `graphCytoscapeOverlay.mount(...)`. See the strategic-rule comment
  // block in that module: no graph lens may implement its own HTML
  // overlay mechanics; lenses may only supply a template.
  //
  // `_sync` is preserved here as a no-op shim so any external diagnostic
  // tool that references the old export does not throw. The shared
  // module owns the live sync.
  function _sync(/* cy */) { /* retired — see strategic rule */ }

  // _dimNativeNodes sets every cy node's `opacity` to 0 via a
  // single-element class so the HTML overlay becomes the visible
  // layer. The node remains hit-testable; we just don't draw it.
  // Edges stay visible so graph topology reads correctly.
  function _dimNativeNodes(cy) {
    if (!cy || _dimmedNativeNodes) return;
    try {
      cy.nodes().style({
        'opacity':        0,
        'text-opacity':   0,
        'border-opacity': 0,
      });
      _dimmedNativeNodes = true;
    } catch (_) { /* swallow */ }
  }

  function _restoreNativeNodes(cy) {
    if (!cy || !_dimmedNativeNodes) return;
    try {
      cy.nodes().removeStyle('opacity text-opacity border-opacity');
    } catch (_) { /* swallow */ }
    _dimmedNativeNodes = false;
  }

  // install — D37r-tranche-B' refactor.
  //
  // Authority's overlay mechanics now flow through the shared
  // `graphCytoscapeOverlay.mount(...)` module
  // (/explorer/assets/js/graph/graph-platform/graph-cytoscape-
  // overlay.js). Authority supplies the per-node template (built from
  // the existing `_buildCard` + `_wireCardClick`) and the per-node
  // key extractor; the shared module owns layer creation, position
  // sync, viewport-event subscription, ResizeObserver, and teardown.
  //
  // Authority's `pointerEvents: 'auto'` opt-in: cards remain clickable
  // (Authority's `_wireCardClick` routes through
  // `_rendererHooks.selectNode`). The shared module always keeps the
  // layer itself `pointer-events: none` so clicks on the layer
  // background fall through to the cy canvas — Cytoscape continues to
  // own pan / drag / marquee on empty space.
  //
  // Authority's `_dimNativeNodes` opt-in remains in this file because
  // it touches cy node style (an Authority-specific visual decision —
  // the overlay layer is the visible card, the cy node is hidden).
  // The shared module never touches cy node styling.
  function install(cy, options) {
    if (!_isActive()) return;
    if (!cy) return;
    options = options || {};
    var mount = options.mount;
    if (!mount && cy.container && typeof cy.container === 'function') {
      mount = cy.container();
    }
    if (!mount) return;

    // Re-installing tears down the previous instance so card state
    // doesn't leak across renders.
    if (_handle) destroy();

    _cy    = cy;
    _mount = mount;

    // Authority's projection-shaped element data drives a `data → DOM`
    // path. Stash the elements so `template.create(node)` can recover
    // the typed-data block (`raw.<kind>`, `isRoot`, etc.) by node id.
    _elementsByKey = {};
    var nodesIn = (options.elements && options.elements.nodes) || [];
    for (var i = 0; i < nodesIn.length; i++) {
      var entry = nodesIn[i];
      if (!entry || !entry.data || !entry.data.id) continue;
      _elementsByKey[String(entry.data.id)] = entry.data;
    }

    var sharedOverlay = window.MIDASExplorerGraph
      && window.MIDASExplorerGraph.graphCytoscapeOverlay;
    if (!sharedOverlay || typeof sharedOverlay.mount !== 'function') {
      // Defensive: if the shared platform module failed to load (for
      // any reason — load-order race, dropped script tag), the gate
      // is open but the overlay cannot be installed. Restore native
      // visibility instead of leaving the canvas blank.
      _elementsByKey = null;
      _cy = null;
      _mount = null;
      return;
    }

    _handle = sharedOverlay.mount(cy, mount, {
      lensId: 'authority',
      template: {
        create: function (node) {
          var id = '';
          try { id = String(node.id() || ''); } catch (_) { id = ''; }
          var data = (id && _elementsByKey && _elementsByKey[id])
            ? _elementsByKey[id]
            : (node.data ? node.data() : {});
          var card = _buildCard(data);
          _wireCardClick(card, id);
          return card;
        },
      },
      keyForNode: function (n) {
        try { return String(n.id() || ''); }
        catch (_) { return ''; }
      },
      // Authority cards are clickable (production right-drawer
      // selection path); the shared module never touches the layer's
      // pointer-events (always `none`).
      pointerEvents: 'auto',
      // Preserve the existing `body.cytoscape-poc-active
      // .midas-cy-overlay-layer { z-index: 6; overflow: hidden; ... }`
      // CSS rule from /explorer/assets/css/cytoscape-html-overlay.css
      // by adding the legacy class to the shared module's layer DIV.
      // This is additive — the shared module's own
      // `graph-cytoscape-overlay-layer` class remains, and the legacy
      // class only contributes the layer-level positioning that
      // Authority's stacking context requires.
      layerClassName: OVERLAY_CLASS,
      // Authority does not currently use selected-class or
      // hover-class state classes on overlay cards. The shared
      // module's subscriptions to cy `select` / `unselect` /
      // `mouseover` / `mouseout` are safe no-ops here because none of
      // Authority's CSS targets `is-selected` / `is-hover` on
      // `.midas-cy-overlay-card`. We leave the defaults
      // (`syncSelected: true`, `syncHover: true`) so future Authority
      // styling can opt into them without code changes.
    });

    _dimNativeNodes(cy);
  }

  // refresh re-renders cards for a new node set (called after a
  // graph reload). Equivalent to destroy + install. Preserved as a
  // public surface so authority-cytoscape-poc.js's existing call
  // path continues to work unchanged.
  function refresh(cy, elements) {
    if (!_isActive() || !cy) return;
    var mount = _mount || (cy.container && cy.container());
    destroy();
    install(cy, { mount: mount, elements: elements });
  }

  // destroy tears down the overlay completely. Idempotent. Delegates
  // listener removal + DOM removal to the shared module's
  // `handle.destroy()`. Authority retains responsibility for
  // restoring cy node opacity (a lens-specific visual concern).
  function destroy() {
    if (_handle && typeof _handle.destroy === 'function') {
      try { _handle.destroy(); } catch (_) { /* swallow */ }
    }
    _handle        = null;
    _elementsByKey = null;
    _mount         = null;
    if (_cy) {
      _restoreNativeNodes(_cy);
    }
    _cy = null;
  }

  window.MIDASExplorerGraph.cytoscapeHtmlOverlay = {
    isActive: _isActive,
    install:  install,
    refresh:  refresh,
    destroy:  destroy,
    // Internals exposed for tests + diagnostics.
    _buildCard:   _buildCard,
    _escHtml:     _escHtml,
    _sync:        _sync,
    _nodeTypeLabel: _nodeTypeLabel,
    OVERLAY_CLASS: OVERLAY_CLASS,
    CARD_CLASS:    CARD_CLASS,
    SYNC_EVENTS:   SYNC_EVENTS,
  };
})();
