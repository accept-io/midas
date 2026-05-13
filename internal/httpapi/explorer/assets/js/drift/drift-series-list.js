// /explorer/assets/js/drift/drift-series-list.js — D32e-impl-1
//
// Renders the left-rail series list for the Drift Analytics panel.
// One row per series. Each row is keyboard-focusable, declares its
// selected state via aria-pressed, and emits a single 'select' event
// (via the supplied onSelect callback) when activated.
//
// Series shape (matches the chart + demo adapter):
//   { id, label, metric, severity, isDemo?, points: [...] }
//
// Render options:
//   { selectedId?: string, onSelect?: function(id), ariaLabel?: string }
//
// Public surface (window.MIDASExplorerDriftSeriesList):
//
//   render(mount, series, options)
//   clear(mount)

(function () {
  'use strict';

  function _escText(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }
  function _escAttr(s) {
    return _escText(s).replace(/"/g, '&quot;');
  }

  function _summary(s) {
    if (!s || !Array.isArray(s.points) || s.points.length === 0) return '—';
    var last = s.points[s.points.length - 1];
    if (typeof last.v !== 'number') return '—';
    var fmt = window.MIDASExplorerDriftChartFormatters || {};
    return (typeof fmt.formatValue === 'function') ? fmt.formatValue(last.v) : String(last.v);
  }

  function _anomalyCount(s) {
    return (s && Array.isArray(s.anomalies)) ? s.anomalies.length : 0;
  }

  function _sevLabel(sev) {
    switch ((sev || '').toLowerCase()) {
      case 'critical': return 'Critical';
      case 'warning':  return 'Warning';
      case 'info':     return 'Info';
      case 'ok':       return 'Stable';
      case 'unknown':  return 'Unknown';
      default:         return 'Info';
    }
  }

  function _rowHTML(s, selected) {
    var sev = s.severity || 'info';
    var sevText = _sevLabel(sev);
    var label = s.label || s.id || 'Series';
    var summary = _summary(s);
    var anomalies = _anomalyCount(s);
    var demoBadge = s.isDemo
      ? '<span class="drift-series-row-demo" aria-label="Synthetic demo data">DEMO</span>'
      : '';
    return '<button type="button" class="drift-series-row drift-series-row-' + _escAttr(sev) +
      (selected ? ' is-selected' : '') + '"' +
      ' data-series-id="' + _escAttr(s.id) + '"' +
      ' aria-pressed="' + (selected ? 'true' : 'false') + '"' +
      ' aria-label="' + _escAttr(label + ' · ' + sevText + ' · latest ' + summary) + '">' +
      '<span class="drift-series-row-sev drift-series-row-sev-' + _escAttr(sev) + '"' +
        ' aria-hidden="true" data-sev="' + _escAttr(sev) + '"></span>' +
      '<span class="drift-series-row-body">' +
        '<span class="drift-series-row-label">' + _escText(label) + '</span>' +
        '<span class="drift-series-row-meta">' +
          '<span class="drift-series-row-sev-text">' + _escText(sevText) + '</span>' +
          (anomalies > 0 ? '<span class="drift-series-row-anomaly-count">' +
                              _escText(String(anomalies) + ' anomal' + (anomalies === 1 ? 'y' : 'ies')) +
                          '</span>' : '') +
          demoBadge +
        '</span>' +
      '</span>' +
      '<span class="drift-series-row-summary">' + _escText(summary) + '</span>' +
    '</button>';
  }

  function render(mount, series, options) {
    if (!mount) return;
    options = options || {};
    var aria = options.ariaLabel || 'Drift series list';
    if (!Array.isArray(series) || series.length === 0) {
      mount.innerHTML =
        '<div class="drift-series-list-empty" role="status" aria-live="polite">' +
          'No drift series for the current selection.' +
        '</div>';
      mount.setAttribute('role', 'list');
      mount.setAttribute('aria-label', aria);
      return;
    }

    var fmt = window.MIDASExplorerDriftChartFormatters || {};
    var rank = (typeof fmt.severityRank === 'function') ? fmt.severityRank : function () { return 0; };
    // Sort by severity descending so critical signals surface first.
    var sorted = series.slice().sort(function (a, b) { return rank(b.severity) - rank(a.severity); });
    var selectedId = options.selectedId || (sorted[0] && sorted[0].id) || null;
    var rowsHTML = sorted.map(function (s) { return _rowHTML(s, s.id === selectedId); }).join('');

    mount.innerHTML = rowsHTML;
    mount.setAttribute('role', 'list');
    mount.setAttribute('aria-label', aria);
    // Detach previous click handler before re-binding so re-render
    // does not double-fire.
    if (mount._driftListHandler) {
      mount.removeEventListener('click', mount._driftListHandler);
      mount._driftListHandler = null;
    }
    if (typeof options.onSelect === 'function') {
      var handler = function (ev) {
        var target = ev.target;
        while (target && target !== mount && !(target.classList && target.classList.contains('drift-series-row'))) {
          target = target.parentNode;
        }
        if (!target || target === mount) return;
        var id = target.dataset && target.dataset.seriesId;
        if (id) {
          try { options.onSelect(id); } catch (_) { /* swallow */ }
        }
      };
      mount._driftListHandler = handler;
      mount.addEventListener('click', handler);
    }
  }

  function clear(mount) {
    if (mount) {
      if (mount._driftListHandler) {
        mount.removeEventListener('click', mount._driftListHandler);
        mount._driftListHandler = null;
      }
      mount.innerHTML = '';
    }
  }

  window.MIDASExplorerDriftSeriesList = {
    render: render,
    clear:  clear,
  };
})();
