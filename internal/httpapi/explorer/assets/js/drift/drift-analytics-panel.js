// /explorer/assets/js/drift/drift-analytics-panel.js - D32e compact summary
//
// Compact Drift Analytics letterbox renderer. The rich chart and
// contribution rail modules remain loaded for the later maximised view,
// but this operational tray renders only a compact projection of the
// shared DriftAnalyticsViewModel.

(function () {
  'use strict';

  var _state = {
    initialised: false,
    expanded: false,
    lastViewModel: null,
    chartWidth: 0,
    chartResizeObserver: null,
    resizeRenderQueued: false,
    hooks: {},
    mountSelectors: {
      compact: '[data-drift-compact-summary]',
      title: '[data-drift-analytics-title]',
      subtitle: '[data-drift-analytics-subtitle]',
      badge: '[data-drift-analytics-demo-badge]'
    }
  };

  function _q(sel) { return document.querySelector(sel); }
  function _adapter() { return window.MIDASExplorerDriftChartAdapter || null; }

  function _activeNodeId() {
    var fn = _state.hooks.getSelectedNodeId;
    if (typeof fn === 'function') {
      try { return fn() || ''; } catch (_) { return ''; }
    }
    return '';
  }

  function _buildViewModel() {
    var adapter = _adapter();
    if (!adapter) return { __loading: true };
    var nodeId = _activeNodeId();
    return nodeId ? adapter.fromGraphNode({ nodeId: nodeId, range: '30d' }) : null;
  }

  function _renderHeader(vm) {
    var titleEl = _q(_state.mountSelectors.title);
    if (titleEl) titleEl.textContent = 'DRIFT ANALYTICS';
    var subtitleEl = _q(_state.mountSelectors.subtitle);
    if (subtitleEl) {
      subtitleEl.textContent = vm && vm.__loading ? 'Loading drift analysis...' :
        (vm && vm.nodeLabel) || 'Select a node to view drift analysis';
      subtitleEl.setAttribute('title', subtitleEl.textContent);
    }
    var badgeEl = _q(_state.mountSelectors.badge);
    if (badgeEl) {
      var demo = _adapter() && typeof _adapter().isDemoData === 'function' && _adapter().isDemoData(vm);
      if (demo) {
        badgeEl.removeAttribute('hidden');
        badgeEl.textContent = 'DEMO DATA';
        badgeEl.setAttribute('title', 'Demo-derived Drift Analytics summary.');
      } else {
        badgeEl.setAttribute('hidden', '');
        badgeEl.textContent = '';
      }
    }
  }

  function _topContribution(vm) {
    var contributions = vm && Array.isArray(vm.contributions) ? vm.contributions : [];
    return contributions[0] || { label: 'No contributor', share: '0%' };
  }

  function _runnerUpContribution(vm) {
    var contributions = vm && Array.isArray(vm.contributions) ? vm.contributions : [];
    return contributions[1] || { label: 'Escalation-rate deviation', share: '26%' };
  }

  function _statusClass(vm) {
    var status = String((vm && vm.scoreStatus) || 'WATCH').toLowerCase();
    if (status.indexOf('breach') >= 0 || status.indexOf('critical') >= 0) return 'critical';
    if (status.indexOf('normal') >= 0 || status.indexOf('ok') >= 0) return 'ok';
    return 'warning';
  }

  function _baselineForIndex(vm, idx) {
    var changeIdx = 16;
    return idx < changeIdx ? (vm.baselineBeforeProfileChange || 0.040) : (vm.baselineAfterProfileChange || 0.052);
  }

  function _pointsForCompact(vm) {
    var points = vm && Array.isArray(vm.points) ? vm.points : [];
    return points.map(function (point, idx) {
      return {
        observed: +point.value,
        expected: _baselineForIndex(vm, idx)
      };
    });
  }

  function _path(points, sx, sy, key) {
    var parts = [];
    for (var i = 0; i < points.length; i++) {
      var v = points[i][key];
      parts.push((i === 0 ? 'M' : 'L') + sx(i).toFixed(2) + ',' + sy(v).toFixed(2));
    }
    return parts.join(' ');
  }

  function _driftBandPath(points, sx, sy) {
    if (!points.length) return '';
    var parts = [];
    for (var i = 0; i < points.length; i++) {
      parts.push((i === 0 ? 'M' : 'L') + sx(i).toFixed(2) + ',' + sy(points[i].observed).toFixed(2));
    }
    for (var j = points.length - 1; j >= 0; j--) {
      parts.push('L' + sx(j).toFixed(2) + ',' + sy(points[j].expected).toFixed(2));
    }
    return parts.join(' ') + ' Z';
  }

  function _compactChart(vm, width) {
    var points = _pointsForCompact(vm);
    if (!points.length) {
      return '<div class="drift-compact-chart-empty" role="status">No drift signal selected.</div>';
    }
    var W = Math.max(360, Math.floor(width || 720));
    var H = 152;
    var pad = { top: 16, right: 32, bottom: 28, left: 46 };
    var plotW = W - pad.left - pad.right;
    var plotH = H - pad.top - pad.bottom;
    var minV = 0;
    var maxV = 0.2;
    function sx(idx) { return pad.left + (idx / Math.max(1, points.length - 1)) * plotW; }
    function sy(value) { return pad.top + plotH - ((value - minV) / (maxV - minV)) * plotH; }

    var yTicks = ['0.000', '0.100', '0.200'];
    var grid = '';
    for (var i = 0; i < yTicks.length; i++) {
      var y = sy(parseFloat(yTicks[i]));
      grid += '<line class="drift-compact-grid" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + y.toFixed(2) + '" y2="' + y.toFixed(2) + '"/>' +
        '<text class="drift-compact-axis-label" x="' + (pad.left - 8) + '" y="' + (y + 4).toFixed(2) + '" text-anchor="end">' + yTicks[i] + '</text>';
    }
    var tickLabels = vm.compactXTicks || ['May 30', 'Jun 07', 'Jun 15', 'Jun 23', 'Jun 29'];
    var xLabels = '';
    for (var t = 0; t < tickLabels.length; t++) {
      var x = pad.left + (t / Math.max(1, tickLabels.length - 1)) * plotW;
      xLabels += '<text class="drift-compact-axis-label" x="' + x.toFixed(2) + '" y="' + (H - 8) + '" text-anchor="middle">' + _escText(tickLabels[t]) + '</text>';
    }

    var last = points[points.length - 1];
    var lastX = sx(points.length - 1);
    var lastY = sy(last.observed);
    var watch = (vm.baselineAfterProfileChange || 0.052) + (vm.watchThresholdOffset || 0.052);
    var breach = (vm.baselineAfterProfileChange || 0.052) + (vm.breachThresholdOffset || 0.100);
    return '<svg class="drift-compact-chart-svg" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="xMinYMin meet" role="img" aria-label="Observed vs expected drift score over the last 30 days">' +
        '<rect class="drift-compact-chart-bg" x="' + pad.left + '" y="' + pad.top + '" width="' + plotW + '" height="' + plotH + '"/>' +
        grid +
        '<line class="drift-compact-threshold drift-compact-watch" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + sy(watch).toFixed(2) + '" y2="' + sy(watch).toFixed(2) + '"/>' +
        '<line class="drift-compact-threshold drift-compact-breach" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + sy(breach).toFixed(2) + '" y2="' + sy(breach).toFixed(2) + '"/>' +
        '<path class="drift-compact-deviation" d="' + _driftBandPath(points, sx, sy) + '"/>' +
        '<path class="drift-compact-expected" d="' + _path(points, sx, sy, 'expected') + '"/>' +
        '<path class="drift-compact-observed" d="' + _path(points, sx, sy, 'observed') + '"/>' +
        '<circle class="drift-compact-latest-dot" cx="' + lastX.toFixed(2) + '" cy="' + lastY.toFixed(2) + '" r="3"/>' +
        '<text class="drift-compact-latest-badge" x="' + Math.max(pad.left, lastX - 34).toFixed(2) + '" y="' + (lastY - 8).toFixed(2) + '">' + _escText(vm.score || '0.146') + '</text>' +
        xLabels +
      '</svg>';
  }

  function _renderCompact(vm) {
    var mount = _q(_state.mountSelectors.compact);
    if (!mount) return;
    if (vm && vm.__loading) {
      mount.innerHTML = '<div class="drift-compact-empty" role="status">' +
        '<div class="drift-compact-empty-title">Loading drift analysis...</div>' +
        '<div class="drift-compact-empty-copy">Preparing the compact Drift Analytics summary.</div>' +
      '</div>';
      return;
    }
    if (!vm) {
      mount.innerHTML = '<div class="drift-compact-empty" role="status">' +
        '<div class="drift-compact-empty-title">Select a node to view drift analysis</div>' +
        '<div class="drift-compact-empty-copy">The compact summary follows the selected graph node.</div>' +
      '</div>';
      return;
    }
    var top = _topContribution(vm);
    var runnerUp = _runnerUpContribution(vm);
    var range = _escText(vm.rangeLabel || 'Last 30 days') + ' &middot; demo';
    var runnerUpLine = 'Next: ' + (runnerUp.label || 'Escalation-rate deviation') + ' - ' + (runnerUp.share || '26%');
    var runnerUpMarkup = 'Next: ' + _escText(runnerUp.label || 'Escalation-rate deviation') + ' &middot; ' + _escText(runnerUp.share || '26%');
    var chartWidth = _state.chartWidth || _estimateChartWidth(mount);
    mount.innerHTML =
      '<section class="drift-compact-layout" aria-label="Compact Drift Analytics summary">' +
        '<div class="drift-compact-card drift-compact-score">' +
          '<div class="drift-compact-card-block drift-compact-card-block--hero">' +
            '<div class="drift-compact-score-row">' +
            '<div class="drift-compact-label">Drift score</div>' +
              '<span class="drift-compact-status drift-compact-status--' + _statusClass(vm) + '">' + _escText(vm.scoreStatus || 'WATCH') + '</span>' +
            '</div>' +
            '<div class="drift-compact-score-value">' + _escText(vm.score || '0.146') + '</div>' +
            '<div class="drift-compact-support drift-compact-number">' + _escText(vm.breachGap || '0.006 below breach') + '</div>' +
            '<div class="drift-compact-range">' + range + '</div>' +
          '</div>' +
        '</div>' +
        '<div class="drift-compact-chart">' +
          '<div class="drift-compact-chart-header">' +
            '<div class="drift-compact-series-title">Observed vs expected</div>' +
            '<div class="drift-compact-legend" aria-label="Chart series">' +
              '<span><i class="drift-legend-observed"></i>Observed</span>' +
              '<span><i class="drift-legend-expected"></i>Expected (declared baseline)</span>' +
            '</div>' +
          '</div>' +
          _compactChart(vm, chartWidth) +
        '</div>' +
        '<div class="drift-compact-card drift-compact-contributor">' +
          '<div class="drift-compact-card-block drift-compact-card-block--hero">' +
            '<div class="drift-compact-label">Top contributor</div>' +
            '<div class="drift-compact-contributor-value" title="' + _escAttr(top.label || 'Authority-path divergence') + '">' + _escText(top.label || 'Authority-path divergence') + '</div>' +
            '<div class="drift-compact-support drift-compact-number">' + _escText((top.share || '49%') + ' of current drift') + '</div>' +
          '</div>' +
          '<div class="drift-compact-divider" aria-hidden="true"></div>' +
          '<div class="drift-compact-card-block drift-compact-card-block--secondary">' +
            '<div class="drift-compact-runner-up drift-compact-number" title="' + _escAttr(runnerUpLine) + '">' + runnerUpMarkup + '</div>' +
            '<div class="drift-compact-evidence">Demo evidence</div>' +
          '</div>' +
        '</div>' +
      '</section>';
    _syncChartWidth(mount);
  }

  function _estimateChartWidth(mount) {
    var w = mount && (mount.clientWidth || mount.offsetWidth);
    if (!w) return 720;
    var side = Math.min(242, Math.max(200, Math.floor(w * 0.24)));
    var right = Math.min(260, Math.max(200, Math.floor(w * 0.25)));
    return Math.max(360, w - side - right - 2);
  }

  function _syncChartWidth(mount) {
    var chart = mount && mount.querySelector ? mount.querySelector('.drift-compact-chart') : null;
    if (!chart) return;
    _measureChartWidth(chart);
    if (typeof ResizeObserver === 'function') {
      if (_state.chartResizeObserver) _state.chartResizeObserver.disconnect();
      _state.chartResizeObserver = new ResizeObserver(function (entries) {
        var entry = entries && entries[0];
        var next = entry && entry.contentRect ? entry.contentRect.width : chart.clientWidth;
        _measureChartWidth(chart, next);
      });
      _state.chartResizeObserver.observe(chart);
    }
  }

  function _measureChartWidth(chart, measured) {
    var next = Math.max(360, Math.floor(measured || _chartContentWidth(chart) || 0));
    if (!next || Math.abs(next - _state.chartWidth) < 2) return;
    _state.chartWidth = next;
    _queueResizeRender();
  }

  function _chartContentWidth(chart) {
    var width = chart.clientWidth || chart.offsetWidth || 0;
    if (!width || typeof getComputedStyle !== 'function') return width;
    var styles = getComputedStyle(chart);
    var left = parseFloat(styles.paddingLeft) || 0;
    var right = parseFloat(styles.paddingRight) || 0;
    return Math.max(0, width - left - right);
  }

  function _queueResizeRender() {
    if (_state.resizeRenderQueued) return;
    _state.resizeRenderQueued = true;
    var run = function () {
      _state.resizeRenderQueued = false;
      if (_state.lastViewModel) _renderCompact(_state.lastViewModel);
    };
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(run);
    else setTimeout(run, 0);
  }

  function render(ctx) {
    void ctx;
    var vm = _buildViewModel();
    _state.lastViewModel = vm;
    _renderHeader(vm);
    _renderCompact(vm);
  }

  function clear() {
    _state.lastViewModel = null;
    if (_state.chartResizeObserver) {
      _state.chartResizeObserver.disconnect();
      _state.chartResizeObserver = null;
    }
    var mount = _q(_state.mountSelectors.compact);
    if (mount) mount.innerHTML = '';
  }

  function setExpanded(expanded) {
    _state.expanded = !!expanded;
    if (_state.expanded) render();
  }

  function setSelectedSeries(seriesId) {
    void seriesId;
    render();
  }

  function init(options) {
    options = options || {};
    _state.hooks = options.hooks || {};
    _state.initialised = true;
    render();
  }

  function _escText(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  function _escAttr(s) {
    return _escText(s).replace(/"/g, '&quot;');
  }

  window.MIDASExplorerDriftAnalytics = {
    init: init,
    render: render,
    clear: clear,
    setExpanded: setExpanded,
    setSelectedSeries: setSelectedSeries,
    _state: _state
  };
})();
