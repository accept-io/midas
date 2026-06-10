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
    requestSeq: 0,
    pendingAbort: null,
    pendingNodeKey: '',
    hooks: {},
    mountSelectors: {
      compact: '[data-drift-compact-summary]',
      title: '[data-drift-analytics-title]',
      subtitle: '[data-drift-analytics-subtitle]',
      badge: '[data-drift-analytics-demo-badge]',
      openAnalysis: '[data-drift-analysis-open]',
      trayBody: '#gmap-evidence-tray-body',
      shell: '[data-drift-analysis-shell]',
      shellContext: '[data-drift-analysis-shell-context]',
      shellBody: '[data-drift-analysis-shell-body]',
      shellClose: '[data-drift-analysis-close]'
    },
    shellOpen: false,
    lastNodeRef: null
  };

  function _q(sel) { return document.querySelector(sel); }
  function _adapter() { return window.MIDASExplorerDriftChartAdapter || null; }
  function _api() { return (window.MIDASExplorerAPI && window.MIDASExplorerAPI.drift) || null; }

  function _activeNodeId() {
    var fn = _state.hooks.getSelectedNodeId;
    if (typeof fn === 'function') {
      try { return fn() || ''; } catch (_) { return ''; }
    }
    return '';
  }

  function _activeNodeRef() {
    var fn = _state.hooks.getSelectedNodeRef;
    if (typeof fn === 'function') {
      try {
        var ref = fn();
        var resolved = _resolveNodeRef(ref);
        if (resolved) return resolved;
      } catch (_) { /* fall through */ }
    }
    return _resolveNodeRef(_activeNodeId());
  }

  function _resolveNodeRef(input) {
    var kind = '';
    var id = '';
    if (input && typeof input === 'object') {
      var source = input.sourceNodeRef || input.nodeRef || input;
      kind = source.kind || input.kind || '';
      id = source.id || input.nodeId || input.id || '';
    } else if (typeof input === 'string') {
      var raw = input.trim();
      var sep = raw.indexOf(':');
      if (sep > 0) {
        kind = raw.slice(0, sep);
        id = raw.slice(sep + 1);
      } else {
        id = raw;
      }
    }
    kind = _backendNodeKind(kind);
    id = String(id || '').trim();
    if (!kind || !id) return null;
    return { kind: kind, id: id };
  }

  function _backendNodeKind(kind) {
    var raw = String(kind || '').trim();
    var k = raw.replace(/[\s_-]+/g, '').toLowerCase();
    var map = {
      bs: 'business_service',
      businessservice: 'business_service',
      business: 'business_service',
      cap: 'capability',
      capability: 'capability',
      proc: 'process',
      process: 'process',
      surface: 'decision_surface',
      decisionsurface: 'decision_surface',
      ai: 'ai_system',
      aisystem: 'ai_system',
      aibinding: 'ai_system_binding',
      aisystembinding: 'ai_system_binding',
      agent: 'agent',
      authorityprofile: 'authority_profile',
      authoritygrant: 'authority_grant'
    };
    return map[k] || '';
  }

  function _buildFallbackViewModel(opts) {
    opts = opts || {};
    var adapter = _adapter();
    if (!adapter) return { __loading: true };
    var nodeId = opts.nodeId || _activeNodeId();
    return nodeId ? adapter.fromGraphNode({
      nodeId: nodeId,
      range: '30d',
      sourceStateLabel: opts.sourceStateLabel || 'Demo evidence'
    }) : null;
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
      } else if (vm && vm.sourceStateLabel) {
        badgeEl.removeAttribute('hidden');
        badgeEl.textContent = String(vm.sourceStateLabel).toUpperCase();
        badgeEl.setAttribute('title', String(vm.sourceStateLabel));
      } else {
        badgeEl.setAttribute('hidden', '');
        badgeEl.textContent = '';
      }
    }
    _syncOpenAnalysisAction(vm);
    if (_state.shellOpen) _renderShell(vm);
  }

  function _canOpenAnalysis(vm) {
    return !!(vm && !vm.__loading && vm.nodeLabel);
  }

  function _syncOpenAnalysisAction(vm) {
    var btn = _q(_state.mountSelectors.openAnalysis);
    if (!btn) return;
    var enabled = _canOpenAnalysis(vm);
    btn.disabled = !enabled;
    btn.setAttribute('aria-disabled', enabled ? 'false' : 'true');
  }

  function _nodeContextLabel(vm) {
    if (!vm || !vm.nodeLabel) return 'Select a graph node to inspect Drift Analysis.';
    var parts = [vm.nodeLabel];
    var ref = _state.lastNodeRef;
    if (ref && ref.kind && ref.id) parts.push(ref.kind + ':' + ref.id);
    return parts.join(' - ');
  }

  function _renderShell(vm) {
    var shell = _q(_state.mountSelectors.shell);
    if (!shell) return;
    var context = _q(_state.mountSelectors.shellContext);
    if (context) {
      context.textContent = _nodeContextLabel(vm);
      context.setAttribute('title', context.textContent);
    }
    var body = _q(_state.mountSelectors.shellBody);
    if (body) {
      body.textContent = 'Detailed chart, source, provenance, composite, and contribution sections arrive in later Drift Analysis tranches. This shell frame does not add backend-backed content yet.';
    }
  }

  function openAnalysisShell() {
    if (!_canOpenAnalysis(_state.lastViewModel)) return false;
    var shell = _q(_state.mountSelectors.shell);
    if (!shell) return false;
    _state.shellOpen = true;
    shell.hidden = false;
    shell.removeAttribute('hidden');
    shell.setAttribute('aria-hidden', 'false');
    shell.classList.add('is-open');
    var trayBody = _q(_state.mountSelectors.trayBody);
    if (trayBody) trayBody.hidden = true;
    _renderShell(_state.lastViewModel);
    var close = _q(_state.mountSelectors.shellClose);
    if (close && typeof close.focus === 'function') {
      try { close.focus(); } catch (_) { /* swallow */ }
    }
    return true;
  }

  function closeAnalysisShell() {
    var shell = _q(_state.mountSelectors.shell);
    _state.shellOpen = false;
    if (!shell) return false;
    shell.classList.remove('is-open');
    shell.setAttribute('aria-hidden', 'true');
    shell.hidden = true;
    var trayBody = _q(_state.mountSelectors.trayBody);
    if (trayBody) trayBody.hidden = false;
    return true;
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

  function _chartStatusClass(status) {
    var s = String(status || '').toLowerCase();
    if (s.indexOf('breach') >= 0 || s.indexOf('critical') >= 0) return 'critical';
    if (s.indexOf('healthy') >= 0 || s.indexOf('normal') >= 0 || s.indexOf('ok') >= 0) return 'ok';
    return 'warning';
  }

  function _formatChartStatus(status) {
    var s = String(status || '').replace(/_/g, ' ').trim();
    return s ? s.toUpperCase() : '';
  }

  function _seriesMap(items) {
    var out = {};
    if (!Array.isArray(items)) return out;
    for (var i = 0; i < items.length; i++) {
      var p = items[i] || {};
      if (p.t == null || typeof p.value !== 'number' || !isFinite(p.value)) continue;
      out[String(p.t)] = p.value;
    }
    return out;
  }

  function _shortDateLabel(t) {
    var d = new Date(t);
    if (isNaN(d.getTime())) return '';
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' });
  }

  function _ticksFromPoints(points, compact) {
    if (!points.length) return [];
    var count = compact ? 5 : 9;
    var labels = [];
    for (var i = 0; i < count; i++) {
      var idx = Math.round((i / Math.max(1, count - 1)) * (points.length - 1));
      var label = points[idx] && points[idx].label;
      if (label) labels.push(label);
    }
    return labels;
  }

  function _sourceClassificationFromBackend(resp, fallback) {
    var src = Object.assign({}, fallback || {}, resp && resp.sourceClassification || {});
    src.observedSeries = src.observedSeries || 'unavailable';
    src.expectedBaseline = src.expectedBaseline || 'unavailable';
    src.thresholds = src.thresholds || 'unavailable';
    src.status = src.status || 'unavailable';
    src.provenance = src.provenance || 'not_available';
    src.compositeScore = 'demo_provisional';
    src.contributionValues = 'demo_provisional';
    src.contributionWeights = 'demo_provisional';
    src.graphOverlay = 'not_implemented';
    return src;
  }

  function _backendViewModel(resp, nodeRef) {
    var vmFactory = window.MIDASExplorerDriftAnalyticsViewModel;
    if (!vmFactory || typeof vmFactory.normalise !== 'function') return null;
    var chart = resp && resp.chart;
    var observed = chart && Array.isArray(chart.observed) ? chart.observed : [];
    var expected = _seriesMap(chart && chart.expected);
    var watch = _seriesMap(chart && chart.watch);
    var breach = _seriesMap(chart && chart.breach);
    if (!resp || resp.dataAvailable !== true || !observed.length) return null;
    var points = [];
    for (var i = 0; i < observed.length; i++) {
      var p = observed[i] || {};
      if (p.t == null || typeof p.value !== 'number' || !isFinite(p.value)) continue;
      var key = String(p.t);
      if (typeof expected[key] !== 'number' || !isFinite(expected[key])) continue;
      points.push({
        id: 'backend-point-' + String(i + 1),
        t: p.t,
        label: _shortDateLabel(p.t),
        value: p.value,
        expected: expected[key],
        watch: watch[key],
        breach: breach[key],
        status: p.status || ''
      });
    }
    if (!points.length) return null;
    var yDomain = chart.yDomain || { min: 0, max: 0.2 };
    var yMax = typeof yDomain.max === 'number' && isFinite(yDomain.max) ? yDomain.max : 0.2;
    var yMin = typeof yDomain.min === 'number' && isFinite(yDomain.min) ? yDomain.min : 0;
    var currentStatus = chart.currentStatus || points[points.length - 1].status || '';
    return vmFactory.normalise({
      nodeLabel: 'Node ' + nodeRef.kind + ':' + nodeRef.id,
      serviceLabel: nodeRef.id,
      points: points,
      values: points.map(function (pt) { return pt.value; }),
      rangeLabel: (resp.range && resp.range.label) || 'Last 30 days',
      sourceClassification: _sourceClassificationFromBackend(resp, vmFactory.demoSourceClassification),
      sourceStateLabel: 'Chart from backend',
      isDemo: false,
      dataAvailable: true,
      yDomain: { min: yMin, max: yMax },
      yTicks: [yMin.toFixed(3), ((yMin + yMax) / 2).toFixed(3), yMax.toFixed(3)],
      xTicks: _ticksFromPoints(points, false),
      compactXTicks: _ticksFromPoints(points, true),
      chartStatus: currentStatus,
      projectionAsOf: resp.projectionAsOf || null,
      provenance: resp.provenance || null
    });
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
        expected: typeof point.expected === 'number' ? point.expected : _baselineForIndex(vm, idx),
        watch: typeof point.watch === 'number' ? point.watch : null,
        breach: typeof point.breach === 'number' ? point.breach : null
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
    var minV = vm.yDomain && typeof vm.yDomain.min === 'number' ? vm.yDomain.min : 0;
    var maxV = vm.yDomain && typeof vm.yDomain.max === 'number' ? vm.yDomain.max : 0.2;
    if (!(maxV > minV)) maxV = minV + 0.2;
    function sx(idx) { return pad.left + (idx / Math.max(1, points.length - 1)) * plotW; }
    function sy(value) { return pad.top + plotH - ((value - minV) / (maxV - minV)) * plotH; }

    var yTicks = Array.isArray(vm.yTicks) && vm.yTicks.length ? vm.yTicks : ['0.000', '0.100', '0.200'];
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
    var hasWatchSeries = points.some(function (p) { return typeof p.watch === 'number'; });
    var hasBreachSeries = points.some(function (p) { return typeof p.breach === 'number'; });
    var watchMarkup = hasWatchSeries
      ? '<path class="drift-compact-threshold drift-compact-watch" d="' + _path(points, sx, sy, 'watch') + '"/>'
      : '<line class="drift-compact-threshold drift-compact-watch" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + sy(watch).toFixed(2) + '" y2="' + sy(watch).toFixed(2) + '"/>';
    var breachMarkup = hasBreachSeries
      ? '<path class="drift-compact-threshold drift-compact-breach" d="' + _path(points, sx, sy, 'breach') + '"/>'
      : '<line class="drift-compact-threshold drift-compact-breach" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + sy(breach).toFixed(2) + '" y2="' + sy(breach).toFixed(2) + '"/>';
    return '<svg class="drift-compact-chart-svg" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="xMinYMin meet" role="img" aria-label="Observed vs expected drift score over the last 30 days">' +
        '<rect class="drift-compact-chart-bg" x="' + pad.left + '" y="' + pad.top + '" width="' + plotW + '" height="' + plotH + '"/>' +
        grid +
        watchMarkup +
        breachMarkup +
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
    var sourceLabel = vm.sourceStateLabel || (vm.isDemo ? 'Demo evidence' : 'Chart from backend');
    var range = _escText(vm.rangeLabel || 'Last 30 days') + ' &middot; ' + _escText(sourceLabel);
    var runnerUpLine = 'Next: ' + (runnerUp.label || 'Escalation-rate deviation') + ' - ' + (runnerUp.share || '26%');
    var runnerUpMarkup = 'Next: ' + _escText(runnerUp.label || 'Escalation-rate deviation') + ' &middot; ' + _escText(runnerUp.share || '26%');
    var chartWidth = _state.chartWidth || _estimateChartWidth(mount);
    var chartStatus = _formatChartStatus(vm.chartStatus);
    var chartStatusMarkup = chartStatus
      ? '<span class="drift-compact-status drift-compact-status--' + _chartStatusClass(chartStatus) + '">' + _escText(chartStatus) + '</span>'
      : '';
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
            chartStatusMarkup +
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

  function _renderViewModel(vm) {
    _state.lastViewModel = vm;
    _renderHeader(vm);
    _renderCompact(vm);
  }

  function _cancelPending() {
    if (_state.pendingAbort && typeof _state.pendingAbort.abort === 'function') {
      try { _state.pendingAbort.abort(); } catch (_) { /* swallow */ }
    }
    _state.pendingAbort = null;
    _state.pendingNodeKey = '';
  }

  function _fallbackForNode(nodeRef, sourceStateLabel) {
    return _buildFallbackViewModel({
      nodeId: nodeRef ? nodeRef.kind + ':' + nodeRef.id : _activeNodeId(),
      sourceStateLabel: sourceStateLabel || 'Demo evidence'
    });
  }

  function _fetchBackend(nodeRef, seq) {
    var api = _api();
    if (!api || typeof api.analytics !== 'function') {
      _renderViewModel(_fallbackForNode(nodeRef, 'Demo evidence'));
      return;
    }
    var aborter = typeof AbortController === 'function' ? new AbortController() : null;
    _state.pendingAbort = aborter;
    api.analytics(nodeRef.kind, nodeRef.id, '30d', { signal: aborter && aborter.signal }).then(function (resp) {
      if (seq !== _state.requestSeq) return;
      if (resp && resp.__status) {
        _renderViewModel(_fallbackForNode(nodeRef, 'Demo evidence'));
        return;
      }
      var vm = _backendViewModel(resp, nodeRef);
      _renderViewModel(vm || _fallbackForNode(nodeRef, 'Demo evidence'));
    }).catch(function (err) {
      if (seq !== _state.requestSeq) return;
      if (err && err.name === 'AbortError') return;
      _renderViewModel(_fallbackForNode(nodeRef, 'Demo evidence'));
    });
  }

  function render(ctx) {
    void ctx;
    var rawNodeId = _activeNodeId();
    var nodeRef = _activeNodeRef();
    _state.lastNodeRef = nodeRef;
    _state.requestSeq += 1;
    var seq = _state.requestSeq;
    _cancelPending();
    if (!nodeRef) {
      _renderViewModel(rawNodeId ? _buildFallbackViewModel({
        nodeId: rawNodeId,
        sourceStateLabel: 'Demo evidence'
      }) : null);
      return;
    }
    _state.pendingNodeKey = nodeRef.kind + ':' + nodeRef.id;
    _renderViewModel({ __loading: true, nodeLabel: 'Node ' + _state.pendingNodeKey });
    _fetchBackend(nodeRef, seq);
  }

  function clear() {
    _state.lastViewModel = null;
    _state.lastNodeRef = null;
    _state.requestSeq += 1;
    _cancelPending();
    if (_state.chartResizeObserver) {
      _state.chartResizeObserver.disconnect();
      _state.chartResizeObserver = null;
    }
    var mount = _q(_state.mountSelectors.compact);
    if (mount) mount.innerHTML = '';
    closeAnalysisShell();
    _syncOpenAnalysisAction(null);
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
    var openBtn = _q(_state.mountSelectors.openAnalysis);
    if (openBtn && !openBtn.__driftAnalysisShellBound) {
      openBtn.__driftAnalysisShellBound = true;
      openBtn.addEventListener('click', openAnalysisShell);
    }
    var closeBtn = _q(_state.mountSelectors.shellClose);
    if (closeBtn && !closeBtn.__driftAnalysisShellBound) {
      closeBtn.__driftAnalysisShellBound = true;
      closeBtn.addEventListener('click', closeAnalysisShell);
    }
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
    openAnalysisShell: openAnalysisShell,
    closeAnalysisShell: closeAnalysisShell,
    _state: _state
  };
})();
