// /explorer/assets/js/graph/authority/authority-graph-overlays.js — D32g-fix-2
//
// Authority Graph drawer content providers + Layers-button lens
// interceptor. NO Authority-specific horizontal toolbar.
//
// D32g-fix-2 (corrective pass on D32g-fix-1):
//
//   • The full-width Authority toolbar (pills + Layers/Info buttons)
//     introduced by D32g-fix-1 is GONE. It created a second menu bar
//     under the main graph header that consumed canvas space and
//     made Authority feel structurally different from Context.
//
//   • Layers control is the EXISTING #gmap-layers-button. When the
//     Authority lens is active, a capture-phase click interceptor
//     redirects the click to the shared right-side drawer's
//     "Posture & Help" tab (where the Authority layer chips already
//     live since D32g-fix-1). The interceptor stops the default
//     Context-Graph layers-panel toggle so the two control surfaces
//     don't open at once.
//
//   • Legend / Help: no extra entry point. The shared drawer's
//     "Posture & Help" tab is the canonical home. Operators reach it
//     by clicking the existing right-rail tab.
//
//   • Risk / gap pills: removed from the canvas chrome entirely.
//     They appear inside the drawer's Diagnostics tab (diagnostic
//     summary) and Posture & Help tab (full Summary section).
//
// Public surface (window.MIDASExplorerGraph.authorityOverlays):
//
//   render(payload)
//     Lens-aware no-op in this tranche — no toolbar to refresh. Kept
//     as a stable entry point so the view's post-paint dispatch
//     continues to work, and so a future tranche can re-add a
//     compact aggregate indicator without re-wiring the view.
//     Calls _ensureLayersButtonInterceptor on first invocation so
//     the lens-aware redirect attaches once.
//
//   clear()
//     Detaches the Layers-button interceptor. Used on full teardown.
//
//   renderLegendInto(mount)
//   renderSummaryInto(mount, payload)
//   renderLayerChipsInto(mount)
//     Drawer content providers — called by the Authority view's
//     "Posture & Help" tab renderer. Unchanged from D32g-fix-1.
//
//   _LAYER_CHIPS / _layerClassFor
//     Test-visible internals.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _adapter() { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityAdapter) || null; }
  function _utils()   { return window.MIDASExplorerUtils || {}; }
  function _drawer()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.drawer) || null; }
  function _store()   { return window.MIDASExplorerStore || null; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _activeLens() {
    var s = _store();
    if (!s || typeof s.getState !== 'function') return null;
    try { return s.getState().selectedGraphLens || null; } catch (_) { return null; }
  }

  // ── Canonical layer-chip definitions ────────────────────────────────────
  //
  // Each chip toggles a CSS class on the .governance-map-body wrapper
  // (the parent of #gmap-canvas). The stylesheet at
  // assets/css/authority-graph.css consumes these classes to hide
  // nodes, edges, and overlay markers.
  // D32h-fix-1 — `defaultOn` flag governs the initial state of each
  // layer chip. The Authority canvas is the *authority spine*; optional
  // governance side-branches (fail-mode policy nodes + escalation
  // target nodes) were dominating the default render and producing the
  // tangled crossings reported on the Showcase service. They remain
  // accessible — operators can toggle them on — but the *default* view
  // is now spine-only, and operators inspect fail-mode + escalation
  // detail in the Authority Workbench / drawer (where the underlying
  // data was always available regardless of the graph-layer state).
  var LAYER_CHIPS = Object.freeze([
    Object.freeze({ id: 'authority-spine', label: 'Authority spine', alwaysOn: true, defaultOn: true }),
    Object.freeze({ id: 'diagnostics',     label: 'Diagnostics',                       defaultOn: true }),
    Object.freeze({ id: 'surface-posture', label: 'Surface posture',                   defaultOn: true }),
    Object.freeze({ id: 'escalation',      label: 'Escalation',                        defaultOn: false }),
    Object.freeze({ id: 'fail-mode',       label: 'Fail-mode policy',                  defaultOn: false }),
  ]);

  function _layerClassFor(chipID, state) {
    if (state === 'off') return 'authority-layer-' + chipID + '-off';
    return '';
  }

  // ── Existing-chrome Layers-button lens interceptor ─────────────────────
  //
  // The existing #gmap-layers-button (in the Context-Graph toolbar
  // header) is the canonical Layers control. In Context Graph mode
  // it opens the existing .gmap-layers-panel popover. In Authority
  // mode we redirect to the shared drawer's "Posture & Help" tab
  // where the Authority layer chips live.
  //
  // The interceptor uses capture-phase + stopImmediatePropagation so
  // the existing wireGmapLayersButton handler does NOT also fire —
  // operators see ONE Layers control opening ONE surface, not both
  // at once.
  var _interceptorInstalled = false;
  var _interceptorHandler = null;

  function _layersButton() { return document.getElementById('gmap-layers-button'); }

  function _ensureLayersButtonInterceptor() {
    if (_interceptorInstalled) return;
    var btn = _layersButton();
    if (!btn) return; // not in the DOM yet — try again on next render()

    _interceptorHandler = function (e) {
      if (_activeLens() !== 'authority') return; // Context: let the existing panel handler run
      e.preventDefault();
      e.stopImmediatePropagation();
      var drawer = _drawer();
      if (!drawer) return;
      if (typeof drawer.open === 'function') {
        try { drawer.open('config'); } catch (_) { /* swallow */ }
      } else if (typeof drawer.setActiveTab === 'function') {
        try { drawer.setActiveTab('config'); } catch (_) { /* swallow */ }
      }
    };
    btn.addEventListener('click', _interceptorHandler, true /* capture */);
    _interceptorInstalled = true;
  }

  function _removeLayersButtonInterceptor() {
    if (!_interceptorInstalled) return;
    var btn = _layersButton();
    if (btn && _interceptorHandler) {
      btn.removeEventListener('click', _interceptorHandler, true);
    }
    _interceptorInstalled = false;
    _interceptorHandler = null;
  }

  // ── Drawer content providers ────────────────────────────────────────────
  //
  // These three functions render INTO arbitrary DOM mount nodes the
  // Authority view's drawer-tab renderer supplies. They are the only
  // surfaces this module emits — there is NO canvas-overlay toolbar.

  function renderLegendInto(mount) {
    if (!mount) return;
    var adapter = _adapter();
    if (!adapter) {
      mount.innerHTML = '';
      return;
    }

    var nodeRows = '';
    for (var i = 0; i < adapter.NODE_KINDS.length; i++) {
      var k = adapter.NODE_KINDS[i];
      var label = adapter.nodeKindLabel(k);
      nodeRows += (
        '<li class="authority-legend-entry">' +
          '<span class="authority-legend-swatch authority-legend-swatch-node" data-node-kind="' + _escHtml(k) + '"></span>' +
          '<span class="authority-legend-label">' + _escHtml(label) + '</span>' +
        '</li>'
      );
    }

    var edgeRows = '';
    for (var j = 0; j < adapter.EDGE_KINDS.length; j++) {
      var ek = adapter.EDGE_KINDS[j];
      var elabel = adapter.edgeKindLabel(ek);
      edgeRows += (
        '<li class="authority-legend-entry">' +
          '<span class="authority-legend-swatch authority-legend-swatch-edge" data-edge-kind="' + _escHtml(ek) + '"></span>' +
          '<span class="authority-legend-label">' + _escHtml(elabel) + '</span>' +
        '</li>'
      );
    }

    var severityRows = (
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-severity" data-diagnostic-severity="critical"></span>' +
        '<span class="authority-legend-label">Error</span>' +
      '</li>' +
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-severity" data-diagnostic-severity="warning"></span>' +
        '<span class="authority-legend-label">Warning</span>' +
      '</li>' +
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-severity" data-diagnostic-severity="info"></span>' +
        '<span class="authority-legend-label">Info</span>' +
      '</li>'
    );

    var postureRows = (
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-posture" data-fmp-status="override"></span>' +
        '<span class="authority-legend-label">Override</span>' +
      '</li>' +
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-posture" data-fmp-status="inherited"></span>' +
        '<span class="authority-legend-label">Inherited</span>' +
      '</li>' +
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-posture" data-fmp-status="missing"></span>' +
        '<span class="authority-legend-label">Missing</span>' +
      '</li>' +
      '<li class="authority-legend-entry">' +
        '<span class="authority-legend-swatch authority-legend-swatch-posture" data-fmp-status="dangling"></span>' +
        '<span class="authority-legend-label">Dangling</span>' +
      '</li>'
    );

    mount.innerHTML = (
      '<div class="authority-drawer-help-intro">' +
        'Authority Graph projects the configuration spine: business service → ' +
        'decision surface → authority profile → authority grant → agent, ' +
        'plus fail-mode policy and escalation routing.' +
      '</div>' +
      '<div class="authority-drawer-legend-columns">' +
        '<section class="authority-drawer-legend-column"><h4>Node kinds</h4><ul class="authority-legend-list">' + nodeRows + '</ul></section>' +
        '<section class="authority-drawer-legend-column"><h4>Edge kinds</h4><ul class="authority-legend-list">' + edgeRows + '</ul></section>' +
        '<section class="authority-drawer-legend-column"><h4>Diagnostic severity</h4><ul class="authority-legend-list">' + severityRows + '</ul></section>' +
        '<section class="authority-drawer-legend-column"><h4>Fail-mode posture</h4><ul class="authority-legend-list">' + postureRows + '</ul></section>' +
      '</div>'
    );
  }

  function renderSummaryInto(mount, payload) {
    if (!mount) return;
    var summary = (payload && payload.summary) || null;
    var diagSummary = (payload && payload.diagnostic_summary) || null;
    if (!summary && !diagSummary) {
      mount.innerHTML = '<div class="authority-drawer-empty">No projection summary available.</div>';
      return;
    }

    var pills = [];
    if (summary) {
      pills.push(_pill('Surfaces', summary.surface_count));
      pills.push(_pill('Active profiles', summary.active_profile_count));
      pills.push(_pill('Active grants', summary.active_grant_count));
      pills.push(_pill('Active agents', summary.active_agent_count));
      if (summary.grants_with_stop_capability > 0) {
        pills.push(_pillEmphasis('Stop authority grants', summary.grants_with_stop_capability));
      }
      if ((summary.surfaces_without_profiles || []).length > 0) {
        pills.push(_pillGap('Surfaces missing profile', summary.surfaces_without_profiles.length));
      }
      if ((summary.profiles_without_grants || []).length > 0) {
        pills.push(_pillGap('Profiles without grants', summary.profiles_without_grants.length));
      }
      if ((summary.grants_without_agents || []).length > 0) {
        pills.push(_pillGap('Grants missing agent', summary.grants_without_agents.length));
      }
      if ((summary.surfaces_without_effective_fail_mode_policy || []).length > 0) {
        pills.push(_pillGap('Surfaces without fail-mode policy', summary.surfaces_without_effective_fail_mode_policy.length));
      }
      if ((summary.policies_missing_active_version || []).length > 0) {
        pills.push(_pillGap('Policies missing active version', summary.policies_missing_active_version.length));
      }
      if ((summary.profiles_with_dangling_escalation_target || []).length > 0) {
        pills.push(_pillGap('Profiles with dangling escalation', summary.profiles_with_dangling_escalation_target.length));
      }
    }
    if (diagSummary) {
      if (diagSummary.critical > 0) pills.push(_pillCritical('Critical diagnostics', diagSummary.critical));
      if (diagSummary.warning > 0)  pills.push(_pillWarning('Warning diagnostics',  diagSummary.warning));
      if (diagSummary.info > 0)     pills.push(_pillInfo('Info diagnostics',        diagSummary.info));
    }

    mount.innerHTML = (
      '<h4 class="authority-drawer-section-title">Summary</h4>' +
      '<div class="authority-drawer-summary-pills">' + pills.join('') + '</div>'
    );
  }

  function renderLayerChipsInto(mount) {
    if (!mount) return;
    if (mount.dataset.layerChipsRendered === '1') {
      _syncLayerChipState(mount);
      return;
    }
    var html = '<h4 class="authority-drawer-section-title">Layers</h4><div class="authority-drawer-layer-chips">';
    for (var i = 0; i < LAYER_CHIPS.length; i++) {
      var chip = LAYER_CHIPS[i];
      var disabledAttr = chip.alwaysOn ? ' disabled aria-disabled="true"' : '';
      // D32h-fix-1 — Honour each chip's `defaultOn`. Spine and posture
      // remain on; governance side-branches (fail-mode, escalation)
      // default off and the corresponding canvas-side `*-off` class is
      // applied below so the initial render matches the chip state.
      var checkedAttr = chip.defaultOn === false ? '' : ' checked';
      html += (
        '<label class="authority-graph-layer-chip" data-layer-id="' + _escHtml(chip.id) + '">' +
          '<input type="checkbox" class="authority-graph-layer-chip-input" data-layer-id="' + _escHtml(chip.id) + '"' + checkedAttr + disabledAttr + '>' +
          '<span class="authority-graph-layer-chip-label">' + _escHtml(chip.label) + '</span>' +
        '</label>'
      );
    }
    html += '</div>';
    mount.innerHTML = html;
    mount.dataset.layerChipsRendered = '1';
    // Apply initial layer-off classes for any chip whose default is off.
    // The chip change-handler below maintains state for subsequent toggles.
    for (var di = 0; di < LAYER_CHIPS.length; di++) {
      var defChip = LAYER_CHIPS[di];
      if (defChip.defaultOn === false) {
        _applyLayerState(defChip.id, 'off');
      }
    }

    mount.addEventListener('change', function (e) {
      if (!e.target || e.target.tagName !== 'INPUT') return;
      var chipID = e.target.getAttribute('data-layer-id');
      if (!chipID) return;
      var checked = !!e.target.checked;
      _applyLayerState(chipID, checked ? 'on' : 'off');
    });
  }

  function _syncLayerChipState(mount) {
    var canvasParent = _layerTargetEl();
    if (!canvasParent || !mount) return;
    var inputs = mount.querySelectorAll('input.authority-graph-layer-chip-input');
    for (var i = 0; i < inputs.length; i++) {
      var chipID = inputs[i].getAttribute('data-layer-id');
      var offClass = _layerClassFor(chipID, 'off');
      if (!offClass) continue;
      inputs[i].checked = !canvasParent.classList.contains(offClass);
    }
  }

  // _layerTargetEl resolves the .governance-map-body ancestor of
  // #gmap-canvas — the layer-off CSS class is toggled there so all
  // descendants (nodes + edges) match the layer-hide selectors.
  function _layerTargetEl() {
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return null;
    var node = canvas.parentNode;
    while (node && node.classList && !node.classList.contains('governance-map-body')) {
      node = node.parentNode;
    }
    return node || (canvas && canvas.parentNode) || null;
  }

  function _applyLayerState(chipID, state) {
    var target = _layerTargetEl();
    if (!target) return;
    var offClass = _layerClassFor(chipID, 'off');
    if (!offClass) return;
    if (state === 'off') {
      target.classList.add(offClass);
    } else {
      target.classList.remove(offClass);
    }
  }

  // ── Pill helpers (used only by drawer summary; no toolbar). ─────────────
  function _pillBase(label, count, cls) {
    return (
      '<span class="' + cls + '" role="status">' +
        '<span class="authority-summary-pill-label">' + _escHtml(label) + '</span>' +
        '<span class="authority-summary-pill-count">' + _escHtml(String(count || 0)) + '</span>' +
      '</span>'
    );
  }
  function _pill(label, count)         { return _pillBase(label, count, 'authority-summary-pill'); }
  function _pillGap(label, count)      { return _pillBase(label, count, 'authority-summary-pill authority-summary-pill-gap'); }
  function _pillCritical(label, count) { return _pillBase(label, count, 'authority-summary-pill authority-summary-pill-critical'); }
  function _pillWarning(label, count)  { return _pillBase(label, count, 'authority-summary-pill authority-summary-pill-warning'); }
  function _pillInfo(label, count)     { return _pillBase(label, count, 'authority-summary-pill authority-summary-pill-info'); }
  function _pillEmphasis(label, count) { return _pillBase(label, count, 'authority-summary-pill authority-summary-pill-emphasis'); }

  // ── Public entry ────────────────────────────────────────────────────────
  //
  // render(payload) is intentionally light in D32g-fix-2 — no toolbar
  // to refresh. It exists so:
  //   • the view's post-paint dispatch into authorityOverlays.render
  //     stays valid (no view-side change needed);
  //   • the Layers-button interceptor is installed lazily (the button
  //     may not have been in the DOM when the module loaded, e.g.
  //     during async asset load ordering).
  // D32h-fix-1 — Layer defaults are applied here (NOT only in
  // renderLayerChipsInto) so the canvas reflects the spine-only
  // default the very first time the Authority graph paints, even
  // before the operator opens the drawer's "Posture & Help" tab.
  // `_layerDefaultsApplied` tracks once-per-mount so subsequent renders
  // never override an explicit operator toggle.
  var _layerDefaultsApplied = false;
  function _applyLayerDefaultsOnce() {
    if (_layerDefaultsApplied) return;
    var target = _layerTargetEl();
    if (!target) return;
    for (var i = 0; i < LAYER_CHIPS.length; i++) {
      var chip = LAYER_CHIPS[i];
      if (chip.defaultOn === false) {
        _applyLayerState(chip.id, 'off');
      }
    }
    _layerDefaultsApplied = true;
  }

  function render(payload) {
    _ensureLayersButtonInterceptor();
    _applyLayerDefaultsOnce();
    void payload;
  }

  function clear() {
    _removeLayersButtonInterceptor();
    _layerDefaultsApplied = false;
  }

  // D32h-fix-2c — Public read API for the current layer state. The
  // Authority view passes the result into computeAuthorityLayout(spec,
  // GMAP, layerState) so visibility is a first-class layout decision
  // rather than a CSS-only side-effect. Each chip id maps to a
  // boolean (true = layer ON / visible). When the canvas-body target
  // is absent (test isolation, very early boot), the function returns
  // the configured `defaultOn` values from LAYER_CHIPS — keeping
  // pre-D32h-fix-2c callers' fallback path stable. `authority-spine`
  // is always true (alwaysOn: true at LAYER_CHIPS).
  //
  // Mirrors the same class-reading logic as _syncLayerChipState
  // (renderLayerChipsInto uses it to keep chip <input>s in sync with
  // the canvas state), so the two paths cannot drift.
  function getLayerState() {
    var target = _layerTargetEl();
    var out = {};
    for (var i = 0; i < LAYER_CHIPS.length; i++) {
      var chip = LAYER_CHIPS[i];
      if (chip.alwaysOn) {
        out[chip.id] = true;
        continue;
      }
      if (!target) {
        out[chip.id] = (chip.defaultOn !== false);
        continue;
      }
      var offClass = _layerClassFor(chip.id, 'off');
      out[chip.id] = !target.classList.contains(offClass);
    }
    return out;
  }

  window.MIDASExplorerGraph.authorityOverlays = {
    render:               render,
    clear:                clear,
    renderLegendInto:     renderLegendInto,
    renderSummaryInto:    renderSummaryInto,
    renderLayerChipsInto: renderLayerChipsInto,
    getLayerState:        getLayerState,
    // Test surface — pinned by Explorer contract tests.
    _LAYER_CHIPS:         LAYER_CHIPS,
    _layerClassFor:       _layerClassFor,
  };
})();
