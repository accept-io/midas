// /explorer/assets/js/graph/authority/authority-diagnostics-panel.js — D32b-impl-2
//
// Renders two Authority Graph panels into their data-* containers:
//
//   • [data-authority-diagnostic-summary] — backend-provided rollup
//     (projection.diagnostic_summary). Shows highest_severity plus
//     info / warning / critical counts and (when present) by-kind
//     counts. No client-side recomputation; if the rollup is missing
//     the panel renders a graceful "No diagnostic summary available."
//
//   • [data-authority-diagnostics] — backend-provided list
//     (projection.diagnostics[]). Each row carries severity, kind,
//     message (when present), and node refs (when present). Order
//     is preserved from the backend; no client-side sort.
//
// External dependencies:
//   window.MIDASExplorerUtils.escHtml — html-safe text emission
//
// Public surface (window.MIDASExplorerGraph.authorityDiagnosticsPanel):
//   render(projection)
//     Reads diagnostic_summary + diagnostics from the Authority
//     projection envelope and paints both DOM containers. Pure DOM
//     — no fetch, no store mutation, no governance derivation.
//   clear()
//     Empties both container DOM children. Used when the panels are
//     about to be hidden (lens switch) so a stale render does not
//     flash on re-show.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _summaryHost() {
    return document.querySelector('[data-authority-diagnostic-summary]');
  }
  function _diagnosticsHost() {
    return document.querySelector('[data-authority-diagnostics]');
  }

  // ── Diagnostic summary ───────────────────────────────────────────────
  //
  // Backend-supplied per-severity rollup. Frontend renders the four
  // canonical values plus an optional by_kind map. If a value is
  // missing or non-numeric, the row prints "—" rather than 0 so the
  // operator can distinguish "no data" from "zero".

  function _coerceCount(v) {
    if (typeof v !== 'number' || !isFinite(v)) return '—';
    return String(v);
  }

  function _renderSummary(summary) {
    var host = _summaryHost();
    if (!host) return;
    if (!summary || typeof summary !== 'object') {
      host.innerHTML = '<div class="authority-diagnostics-summary-empty">No diagnostic summary available.</div>';
      return;
    }
    var sev = summary.highest_severity || 'none';
    var rows = [
      ['Highest severity', sev],
      ['Info',             _coerceCount(summary.info)],
      ['Warning',          _coerceCount(summary.warning)],
      ['Critical',         _coerceCount(summary.critical)],
    ];
    var rowsHtml = rows.map(function (pair) {
      return '<div class="authority-diagnostics-summary-row">' +
               '<span class="authority-diagnostics-summary-key">' + _escHtml(pair[0]) + '</span>' +
               '<span class="authority-diagnostics-summary-val authority-diagnostics-severity-' + _escHtml(String(pair[1])) + '">' +
                 _escHtml(String(pair[1])) +
               '</span>' +
             '</div>';
    }).join('');

    var byKindHtml = '';
    if (summary.by_kind && typeof summary.by_kind === 'object') {
      var keys = Object.keys(summary.by_kind);
      keys.sort();
      if (keys.length) {
        byKindHtml =
          '<div class="authority-diagnostics-summary-bykind">' +
            '<div class="authority-diagnostics-summary-bykind-title">By kind</div>' +
            keys.map(function (k) {
              return '<div class="authority-diagnostics-summary-bykind-row">' +
                       '<span class="authority-diagnostics-summary-bykind-key">' + _escHtml(k) + '</span>' +
                       '<span class="authority-diagnostics-summary-bykind-val">' + _escHtml(String(summary.by_kind[k])) + '</span>' +
                     '</div>';
            }).join('') +
          '</div>';
      }
    }

    host.innerHTML =
      '<div class="authority-diagnostics-summary-title">Diagnostic summary</div>' +
      '<div class="authority-diagnostics-summary-rows">' + rowsHtml + '</div>' +
      byKindHtml;
  }

  // ── Diagnostics list ─────────────────────────────────────────────────
  //
  // Each row carries the backend-supplied severity / kind / message
  // verbatim. node_refs (if present) render as a small kind:id chip
  // list so the operator can scan which graph nodes the diagnostic
  // references without re-parsing the projection.

  function _renderDiagnostics(diagnostics) {
    var host = _diagnosticsHost();
    if (!host) return;
    if (!Array.isArray(diagnostics) || diagnostics.length === 0) {
      host.innerHTML =
        '<div class="authority-diagnostics-list-title">Diagnostics</div>' +
        '<div class="authority-diagnostics-list-empty">No diagnostics for this service.</div>';
      return;
    }
    var rowsHtml = diagnostics.map(function (d) {
      if (!d || typeof d !== 'object') return '';
      var sev  = d.severity || 'info';
      var kind = d.kind     || 'diagnostic';
      var msg  = d.message  || '';
      var refs = Array.isArray(d.node_refs) ? d.node_refs : [];
      var refsHtml = '';
      if (refs.length) {
        refsHtml = '<div class="authority-diagnostics-row-refs">' +
          refs.map(function (r) {
            if (!r || typeof r !== 'object') return '';
            return '<span class="authority-diagnostics-row-ref">' +
                     _escHtml(String(r.kind || '')) + ':' + _escHtml(String(r.id || '')) +
                   '</span>';
          }).join('') +
        '</div>';
      }
      return '<div class="authority-diagnostics-row authority-diagnostics-row-' + _escHtml(sev) + '">' +
               '<div class="authority-diagnostics-row-head">' +
                 '<span class="authority-diagnostics-row-severity authority-diagnostics-severity-' + _escHtml(sev) + '">' + _escHtml(sev) + '</span>' +
                 '<span class="authority-diagnostics-row-kind">' + _escHtml(kind) + '</span>' +
               '</div>' +
               (msg ? '<div class="authority-diagnostics-row-msg">' + _escHtml(msg) + '</div>' : '') +
               refsHtml +
             '</div>';
    }).join('');
    host.innerHTML =
      '<div class="authority-diagnostics-list-title">Diagnostics (' + diagnostics.length + ')</div>' +
      '<div class="authority-diagnostics-list-rows">' + rowsHtml + '</div>';
  }

  // ── Public render entry-point ────────────────────────────────────────
  function render(projection) {
    if (!projection || typeof projection !== 'object') {
      _renderSummary(null);
      _renderDiagnostics(null);
      return;
    }
    _renderSummary(projection.diagnostic_summary || projection.diagnosticSummary || null);
    _renderDiagnostics(projection.diagnostics || []);
  }

  function clear() {
    var s = _summaryHost();
    if (s) s.innerHTML = '';
    var d = _diagnosticsHost();
    if (d) d.innerHTML = '';
  }

  window.MIDASExplorerGraph.authorityDiagnosticsPanel = {
    render: render,
    clear:  clear,
  };
})();
