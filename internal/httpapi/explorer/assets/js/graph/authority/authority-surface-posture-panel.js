// /explorer/assets/js/graph/authority/authority-surface-posture-panel.js — D32b-impl-2
//
// Renders the Authority Graph surface posture panel into the
// [data-authority-surface-posture] container.
//
// Backend source: projection.surface_posture[] (D31m). Each posture
// record is a deterministic per-emitted-surface status carrying the
// authority/profile/grant/agent/fail-mode/escalation axes plus a
// diagnostic-kinds set and a highest-severity rollup. The frontend
// renders these verbatim — no status is recomputed from the raw
// node/edge topology, and no sort is applied (the backend already
// orders by surface id ascending).
//
// Interaction: clicking a posture row attempts to select + focus the
// corresponding decision_surface node in the graph. The Authority
// view's renderer keys node ids as `decision_surface:<id>` (see
// authority-graph-view.js _refKey), so the row's data-surface-id is
// composed to that key for graph selection. If a graph-selection /
// focus API is unavailable, the click is a no-op rather than a noisy
// console error.
//
// External dependencies:
//   window.MIDASExplorerUtils.escHtml — html-safe text emission
//   window.MIDASExplorerGraph.selection / .camera — optional graph
//     selection + focus APIs
//   window.MIDASExplorerGraph.authorityInspector — optional inspector
//     handoff for the selected surface
//
// Public surface (window.MIDASExplorerGraph.authoritySurfacePosturePanel):
//   render(projection)
//     Reads projection.surface_posture[] and paints the container.
//   clear()
//     Empties the container.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _host() {
    return document.querySelector('[data-authority-surface-posture]');
  }
  function _selection() { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.selection) || null; }
  function _camera()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.camera) || null; }
  function _inspector() { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityInspector) || null; }

  function _coerceCount(v) {
    if (typeof v !== 'number' || !isFinite(v)) return '—';
    return String(v);
  }

  // Status axis renderer — produces a small kind:value chip. Empty /
  // missing axis values fall through to the literal "—" so a sparse
  // surface_posture record still produces a complete-looking row.
  function _statusChip(label, value) {
    var v = value || '—';
    var cls = 'authority-posture-chip authority-posture-chip-' + _escHtml(label.toLowerCase());
    return '<span class="' + cls + '" data-axis="' + _escHtml(label) + '">' +
             '<span class="authority-posture-chip-key">' + _escHtml(label) + '</span>' +
             '<span class="authority-posture-chip-val">' + _escHtml(String(v)) + '</span>' +
           '</span>';
  }

  function _renderRow(p) {
    if (!p || typeof p !== 'object') return '';
    var sid = p.surface_id || p.surfaceId || '';
    var sev = p.highest_severity || 'none';
    var diagKinds = Array.isArray(p.diagnostic_kinds) ? p.diagnostic_kinds : [];
    var diagKindsHtml = diagKinds.length
      ? '<div class="authority-posture-row-diagkinds">' +
          diagKinds.map(function (k) {
            return '<span class="authority-posture-row-diagkind">' + _escHtml(String(k)) + '</span>';
          }).join('') +
        '</div>'
      : '';
    return '<button type="button" class="authority-posture-row authority-posture-row-' + _escHtml(sev) + '"' +
             ' data-surface-id="' + _escHtml(sid) + '"' +
             ' aria-label="Surface posture for ' + _escHtml(sid) + '">' +
             '<div class="authority-posture-row-head">' +
               '<span class="authority-posture-row-surface">' + _escHtml(sid) + '</span>' +
               '<span class="authority-posture-row-severity authority-diagnostics-severity-' + _escHtml(sev) + '">' + _escHtml(sev) + '</span>' +
             '</div>' +
             '<div class="authority-posture-row-axes">' +
               _statusChip('Authority',     p.authority_status) +
               _statusChip('Profile',       p.profile_status) +
               _statusChip('Grant',         p.grant_status) +
               _statusChip('Agent',         p.agent_status) +
               _statusChip('FailMode',      p.fail_mode_policy_status) +
               _statusChip('Escalation',    p.escalation_status) +
             '</div>' +
             '<div class="authority-posture-row-paths">' +
               '<span class="authority-posture-row-paths-key">Paths</span>' +
               '<span class="authority-posture-row-paths-val">' +
                 'complete ' + _coerceCount(p.complete_paths) +
                 ' / incomplete ' + _coerceCount(p.incomplete_paths) +
               '</span>' +
             '</div>' +
             diagKindsHtml +
           '</button>';
  }

  function render(projection) {
    var host = _host();
    if (!host) return;
    var postures = projection && Array.isArray(projection.surface_posture)
      ? projection.surface_posture
      : [];
    if (postures.length === 0) {
      host.innerHTML =
        '<div class="authority-posture-title">Surface posture</div>' +
        '<div class="authority-posture-empty">No surface posture for this service.</div>';
      _detachClicks();
      return;
    }
    host.innerHTML =
      '<div class="authority-posture-title">Surface posture (' + postures.length + ')</div>' +
      '<div class="authority-posture-rows">' +
        postures.map(_renderRow).join('') +
      '</div>';
    _attachClicks();
  }

  // _attachClicks / _detachClicks — single delegated click handler on
  // the posture rows. The handler resolves the underlying decision-
  // surface node id (the Authority view keys node DOM ids as
  // `decision_surface:<id>`) and routes the click through whichever
  // graph-selection API is present. Where none is present, the click
  // is a quiet no-op so the panel remains useful as a read-only
  // summary.
  var _delegate = null;
  function _attachClicks() {
    var host = _host();
    if (!host) return;
    _detachClicks();
    _delegate = function (ev) {
      var target = ev.target;
      while (target && target !== host && !(target.classList && target.classList.contains('authority-posture-row'))) {
        target = target.parentNode;
      }
      if (!target || target === host) return;
      var sid = target.dataset && target.dataset.surfaceId;
      if (!sid) return;
      _selectSurface(sid);
    };
    host.addEventListener('click', _delegate);
  }
  function _detachClicks() {
    var host = _host();
    if (!host || !_delegate) return;
    host.removeEventListener('click', _delegate);
    _delegate = null;
  }

  function _selectSurface(surfaceId) {
    // The Authority view paints decision_surface nodes with DOM id
    // dataset.nodeId === 'decision_surface:<id>'. Use that key when
    // calling graph-selection / inspector.
    var nodeId = 'decision_surface:' + surfaceId;
    var sel = _selection();
    if (sel && typeof sel.setSelected === 'function') {
      try { sel.setSelected(nodeId); } catch (_) { /* swallow */ }
    }
    // Toggle .selected on the DOM node directly so the visual lines
    // up even if the selection module is busy. Best-effort only.
    var canvas = document.getElementById('gmap-canvas');
    if (canvas) {
      var nodes = canvas.querySelectorAll('.gmap-node');
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i];
        n.classList.toggle('selected', n.dataset && n.dataset.nodeId === nodeId);
      }
    }
    var insp = _inspector();
    if (insp && typeof insp.selectNode === 'function') {
      try { insp.selectNode(nodeId); } catch (_) { /* swallow */ }
    }
    // Best-effort focus — if a focus API exists on the graph camera,
    // hand off the node id. If not, the panel selection alone is
    // sufficient for D32b-impl-2.
    var cam = _camera();
    if (cam && typeof cam.focusOnNode === 'function') {
      try { cam.focusOnNode(nodeId); } catch (_) { /* swallow */ }
    }
  }

  function clear() {
    var host = _host();
    if (host) host.innerHTML = '';
    _detachClicks();
  }

  window.MIDASExplorerGraph.authoritySurfacePosturePanel = {
    render: render,
    clear:  clear,
  };
})();
