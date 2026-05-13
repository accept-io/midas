// /explorer/assets/js/drift/drift-analytics-panel.js — D32e-impl-1
//
// Drift Analytics panel — replaces the inside of the existing Runtime
// Evidence letterbox (#gmap-evidence-tray) with a richer chart-plus-
// series-list layout. The existing tray header / collapse-toggle /
// shell remains so focus-mode behaviour, tray-state persistence, and
// any prior selection/notification wiring continue to work; only the
// Drift tab's panel body content is owned by this module.
//
// Public surface (window.MIDASExplorerDriftAnalytics):
//
//   init(options)
//     Wire the panel to its mount + hook bundle. Idempotent.
//     options = {
//       hooks: {
//         getServiceId:        () => string | null,
//         getSelectedNodeId:   () => string | null,
//       },
//     }
//
//   render(context)
//     Re-render with the current selection. `context` is optional;
//     when omitted the panel reads from hooks. When the hook bundle
//     is absent (test isolation), the panel renders an empty state.
//
//   clear()
//     Wipes the chart + series list. Used on lens-switch / mode-exit.
//
//   setExpanded(expanded)
//     Mirror of the surrounding tray's expand state. Stored locally
//     so the panel can avoid expensive re-renders when collapsed.
//
//   setSelectedSeries(seriesId)
//     Manually select a series (programmatic equivalent of a series-
//     list row click).

(function () {
  'use strict';

  var _state = {
    initialised:        false,
    expanded:           false,
    selectedSeriesId:   null,
    lastResult:         null,
    hooks:              {},
    mountSelectors: {
      chart:     '[data-drift-analytics-chart]',
      list:      '[data-drift-analytics-series-list]',
      title:     '[data-drift-analytics-title]',
      subtitle:  '[data-drift-analytics-subtitle]',
      badge:     '[data-drift-analytics-demo-badge]',
      severity:  '[data-drift-analytics-severity-badge]',
    },
  };

  function _q(sel) { return document.querySelector(sel); }
  function _adapter() {
    return (window.MIDASExplorerDriftChartAdapter) || null;
  }
  function _chart() {
    return (window.MIDASExplorerDriftSeriesChart) || null;
  }
  function _list() {
    return (window.MIDASExplorerDriftSeriesList) || null;
  }

  function _activeServiceId() {
    var fn = _state.hooks.getServiceId;
    if (typeof fn === 'function') {
      try { return fn() || ''; } catch (_) { return ''; }
    }
    return '';
  }

  function _activeNodeId() {
    var fn = _state.hooks.getSelectedNodeId;
    if (typeof fn === 'function') {
      try { return fn() || ''; } catch (_) { return ''; }
    }
    return '';
  }

  // _buildResult — prefers a real-data adapter (future: backend
  // /v1/drift/series points keyed by service/node), falls back to
  // the demo adapter when no real series id is resolvable. Today
  // every path goes to the demo adapter; the production path will
  // be added once the Explorer surfaces service→series mappings.
  function _buildResult() {
    var adapter = _adapter();
    if (!adapter) return null;
    var nodeId = _activeNodeId();
    if (nodeId) return adapter.fromGraphNode({ nodeId: nodeId, range: '7d' });
    var svcId = _activeServiceId();
    return adapter.fromServiceContext({ serviceId: svcId, range: '7d' });
  }

  function _topSeverity(result) {
    if (!result || !Array.isArray(result.series) || result.series.length === 0) return 'info';
    var fmt = window.MIDASExplorerDriftChartFormatters || {};
    var rank = (typeof fmt.severityRank === 'function') ? fmt.severityRank : function () { return 0; };
    var top = result.series[0];
    for (var i = 1; i < result.series.length; i++) {
      if (rank(result.series[i].severity) > rank(top.severity)) top = result.series[i];
    }
    return top.severity || 'info';
  }

  function _renderHeader(result) {
    var titleEl = _q(_state.mountSelectors.title);
    if (titleEl) titleEl.textContent = 'Drift Analytics';

    var subtitleEl = _q(_state.mountSelectors.subtitle);
    if (subtitleEl) {
      var nodeId = _activeNodeId();
      var svcId = _activeServiceId();
      if (nodeId) {
        subtitleEl.textContent = 'Node ' + nodeId;
      } else if (svcId) {
        subtitleEl.textContent = 'Service ' + svcId;
      } else {
        subtitleEl.textContent = 'Select a business service or graph node';
      }
    }

    var sevEl = _q(_state.mountSelectors.severity);
    if (sevEl) {
      var sev = _topSeverity(result);
      sevEl.textContent = sev === 'critical' ? 'Critical' :
                          sev === 'warning'  ? 'Warning'  :
                          sev === 'unknown'  ? 'Unknown'  :
                          sev === 'ok'       ? 'Stable'   : 'Info';
      sevEl.className = 'drift-analytics-severity-badge drift-analytics-severity-' + sev;
    }

    var badgeEl = _q(_state.mountSelectors.badge);
    if (badgeEl) {
      var adapter = _adapter();
      var demo = adapter && typeof adapter.isDemoData === 'function' && adapter.isDemoData(result);
      if (demo) {
        badgeEl.removeAttribute('hidden');
        badgeEl.setAttribute(
          'title',
          'Synthetic illustrative data. The Drift Analytics panel switches to real /v1/drift/series points when the selected service has a resolvable drift series id.'
        );
        badgeEl.textContent = 'DEMO DATA';
      } else {
        badgeEl.setAttribute('hidden', '');
        badgeEl.textContent = '';
      }
    }
  }

  function _renderChart(result) {
    var mount = _q(_state.mountSelectors.chart);
    if (!mount) return;
    var chart = _chart();
    if (!chart) return;
    if (!result || !Array.isArray(result.series) || result.series.length === 0) {
      chart.render(mount, null, { ariaLabel: 'No drift series selected' });
      return;
    }
    var selected = null;
    for (var i = 0; i < result.series.length; i++) {
      if (result.series[i].id === _state.selectedSeriesId) { selected = result.series[i]; break; }
    }
    if (!selected) selected = _topPickFromResult(result);
    chart.render(mount, selected, {
      ariaLabel: 'Drift signal for ' + (selected.label || selected.id),
      showAnomalies: true,
      showBaseline:  true,
      height:        220,
    });
  }

  function _topPickFromResult(result) {
    var fmt = window.MIDASExplorerDriftChartFormatters || {};
    var rank = (typeof fmt.severityRank === 'function') ? fmt.severityRank : function () { return 0; };
    var sorted = result.series.slice().sort(function (a, b) { return rank(b.severity) - rank(a.severity); });
    return sorted[0];
  }

  function _renderList(result) {
    var mount = _q(_state.mountSelectors.list);
    if (!mount) return;
    var list = _list();
    if (!list) return;
    if (!result || !Array.isArray(result.series) || result.series.length === 0) {
      list.render(mount, [], { ariaLabel: 'Drift series list' });
      return;
    }
    if (!_state.selectedSeriesId) {
      _state.selectedSeriesId = _topPickFromResult(result).id;
    }
    list.render(mount, result.series, {
      selectedId: _state.selectedSeriesId,
      ariaLabel:  'Drift series list',
      onSelect:   function (id) {
        if (_state.selectedSeriesId === id) return;
        _state.selectedSeriesId = id;
        render();
      },
    });
  }

  function render(ctx) {
    void ctx; // ctx unused today; hooks own selection state
    var result = _buildResult();
    _state.lastResult = result;
    _renderHeader(result);
    _renderChart(result);
    _renderList(result);
  }

  function clear() {
    _state.lastResult = null;
    _state.selectedSeriesId = null;
    var chartMount = _q(_state.mountSelectors.chart);
    var listMount  = _q(_state.mountSelectors.list);
    if (chartMount && _chart()) _chart().clear(chartMount);
    if (listMount  && _list())  _list().clear(listMount);
  }

  function setExpanded(expanded) {
    _state.expanded = !!expanded;
    if (_state.expanded) render();
  }

  function setSelectedSeries(seriesId) {
    _state.selectedSeriesId = seriesId || null;
    render();
  }

  function init(options) {
    options = options || {};
    _state.hooks = options.hooks || {};
    if (_state.initialised) {
      render();
      return;
    }
    _state.initialised = true;
    render();
  }

  window.MIDASExplorerDriftAnalytics = {
    init:               init,
    render:             render,
    clear:              clear,
    setExpanded:        setExpanded,
    setSelectedSeries:  setSelectedSeries,
    // Exposed for tests / advanced consumers only.
    _state:             _state,
  };
})();
