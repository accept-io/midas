// /explorer/assets/js/drift/drift-chart-formatters.js — D32e-impl-1
//
// Pure formatters for the Drift Analytics panel + chart. No DOM,
// no fetch, no module state. Each helper takes its inputs and
// returns a primitive or a small POJO.
//
// Exposed on window.MIDASExplorerDriftChartFormatters so the chart,
// series list, and panel modules share a single formatting source.

(function () {
  'use strict';

  // formatTimestamp — short HH:MM if the date is within the last 24h,
  // otherwise MMM DD. The chart's x-axis ticks use this format.
  function formatTimestamp(input) {
    if (input == null) return '';
    var d = input instanceof Date ? input : new Date(input);
    if (isNaN(d.getTime())) return '';
    var now = new Date();
    var sameDay = d.getFullYear() === now.getFullYear() &&
                  d.getMonth() === now.getMonth() &&
                  d.getDate() === now.getDate();
    if (sameDay) {
      return pad2(d.getHours()) + ':' + pad2(d.getMinutes());
    }
    var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    return months[d.getMonth()] + ' ' + pad2(d.getDate());
  }

  // formatTimestampLong — ISO-style YYYY-MM-DD HH:MM. Used by tooltips
  // / row meta where unambiguous wall-clock format is preferable.
  function formatTimestampLong(input) {
    if (input == null) return '';
    var d = input instanceof Date ? input : new Date(input);
    if (isNaN(d.getTime())) return '';
    return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate()) +
      ' ' + pad2(d.getHours()) + ':' + pad2(d.getMinutes());
  }

  // formatValue — concise number formatter. 0.012345 → "0.012",
  // 12345 → "12.3k". Drift magnitudes are typically small positive
  // floats; chart tooltips use this for y-axis values.
  function formatValue(input) {
    if (input == null || input === '') return '—';
    var n = +input;
    if (!isFinite(n)) return '—';
    if (Math.abs(n) >= 1e9)   return (n / 1e9).toFixed(1) + 'B';
    if (Math.abs(n) >= 1e6)   return (n / 1e6).toFixed(1) + 'M';
    if (Math.abs(n) >= 1e3)   return (n / 1e3).toFixed(1) + 'k';
    if (Math.abs(n) >= 1)     return n.toFixed(2);
    if (Math.abs(n) >= 0.001) return n.toFixed(3);
    return n.toExponential(2);
  }

  // severityRank — total order for severity strings. Used to sort
  // series rows in the list (highest severity first) and to bucket
  // chart hover-state colours.
  function severityRank(severity) {
    switch ((severity || '').toLowerCase()) {
      case 'critical': return 3;
      case 'warning':  return 2;
      case 'info':     return 1;
      default:         return 0;
    }
  }

  function pad2(n) { return n < 10 ? '0' + n : String(n); }

  // classifyPointSeverity — derive a coarse severity bucket from the
  // point's backend `status` string. The classification mirrors the
  // existing drift-workbench status taxonomy.
  function classifyPointSeverity(status) {
    var s = String(status || '').toLowerCase();
    if (s === 'drift' || s === 'severe' || s === 'critical') return 'critical';
    if (s === 'warning' || s === 'elevated' || s === 'borderline') return 'warning';
    if (s === 'unknown_insufficient_data' || s === 'unknown_detector_error') return 'unknown';
    if (s === 'ok' || s === 'no_drift') return 'ok';
    return 'info';
  }

  window.MIDASExplorerDriftChartFormatters = {
    formatTimestamp:       formatTimestamp,
    formatTimestampLong:   formatTimestampLong,
    formatValue:           formatValue,
    severityRank:          severityRank,
    classifyPointSeverity: classifyPointSeverity,
  };
})();
