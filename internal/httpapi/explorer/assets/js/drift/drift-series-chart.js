// /explorer/assets/js/drift/drift-series-chart.js - D32e-tranche-1
//
// SVG renderer for the Drift Analytics view model.

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

  function _pointPath(points, sx, sy) {
    var parts = [];
    for (var i = 0; i < points.length; i++) {
      parts.push((i === 0 ? 'M' : 'L') + sx(i).toFixed(2) + ',' + sy(points[i].value).toFixed(2));
    }
    return parts.join(' ');
  }

  function _renderEmpty(mount, ariaLabel) {
    mount.innerHTML = '<div class="drift-series-chart-empty" role="status" aria-live="polite">No drift signal selected.</div>';
    mount.setAttribute('aria-label', ariaLabel || 'No drift signal selected');
  }

  function render(mount, viewModel, options) {
    if (!mount) return;
    options = options || {};
    var points = viewModel && Array.isArray(viewModel.points) ? viewModel.points : [];
    if (points.length === 0) {
      _renderEmpty(mount, options.ariaLabel);
      return;
    }

    var W = 960;
    var H = options.height || 260;
    var pad = { top: 18, right: 42, bottom: 44, left: 56 };
    var plotW = W - pad.left - pad.right;
    var plotH = H - pad.top - pad.bottom;
    var minV = 0;
    var maxV = 0.2;
    var spanV = maxV - minV;
    function sx(idx) { return pad.left + (idx / Math.max(1, points.length - 1)) * plotW; }
    function sy(value) { return pad.top + plotH - ((value - minV) / spanV) * plotH; }

    var grid = '';
    var labelsY = '';
    var yTicks = viewModel.yTicks || [];
    for (var i = 0; i < yTicks.length; i++) {
      var v = parseFloat(yTicks[i]);
      var y = sy(v);
      grid += '<line class="drift-series-chart-grid" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + y.toFixed(2) + '" y2="' + y.toFixed(2) + '"/>';
      labelsY += '<text class="drift-series-chart-axis-label" x="' + (pad.left - 8) + '" y="' + (y + 4).toFixed(2) + '" text-anchor="end">' + _escText(yTicks[i]) + '</text>';
    }

    var labelsX = '';
    var xTicks = viewModel.xTicks || [];
    for (var j = 0; j < xTicks.length; j++) {
      var x = pad.left + (j / Math.max(1, xTicks.length - 1)) * plotW;
      labelsX += '<text class="drift-series-chart-axis-label" x="' + x.toFixed(2) + '" y="' + (H - 14) + '" text-anchor="middle">' + _escText(xTicks[j]) + '</text>';
    }

    var actualPath = _pointPath(points, sx, sy);
    var areaPath = actualPath + ' L' + sx(points.length - 1).toFixed(2) + ',' + sy(0).toFixed(2) + ' L' + sx(0).toFixed(2) + ',' + sy(0).toFixed(2) + ' Z';
    var baselineY = sy(viewModel.baseline || 0.052).toFixed(2);
    var watchY = sy(viewModel.watchThreshold || 0.104).toFixed(2);
    var breachY = sy(viewModel.breachThreshold || 0.152).toFixed(2);
    var last = points[points.length - 1];
    var lastX = sx(points.length - 1).toFixed(2);
    var lastY = sy(last.value).toFixed(2);

    var events = '';
    var modelEvents = viewModel.events || [];
    for (var e = 0; e < modelEvents.length; e++) {
      var ex = pad.left + ((e + 1) / (modelEvents.length + 1)) * plotW;
      events += '<g class="drift-series-event" data-drift-event-id="' + _escAttr(modelEvents[e].id) + '">' +
        '<line class="drift-series-event-line" x1="' + ex.toFixed(2) + '" x2="' + ex.toFixed(2) + '" y1="' + pad.top + '" y2="' + (H - pad.bottom) + '"/>' +
        '<text class="drift-series-event-label" x="' + ex.toFixed(2) + '" y="' + (H - 24) + '" text-anchor="middle">' + _escText(modelEvents[e].label) + '</text>' +
        '<text class="drift-series-event-date" x="' + ex.toFixed(2) + '" y="' + (H - 8) + '" text-anchor="middle">' + _escText(modelEvents[e].date) + '</text>' +
      '</g>';
    }

    mount.innerHTML =
      '<svg class="drift-series-chart-svg drift-series-chart-severity-warning" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" role="img" aria-label="' + _escAttr(options.ariaLabel || 'Observed vs. expected - drift is the deviation, not the level') + '">' +
        '<defs><linearGradient id="drift-series-area-gradient" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stop-color="var(--drift-chart-gradient-start)"/><stop offset="100%" stop-color="var(--drift-chart-gradient-end)"/></linearGradient></defs>' +
        '<rect class="drift-series-chart-bg" x="' + pad.left + '" y="' + pad.top + '" width="' + plotW + '" height="' + plotH + '"/>' +
        '<rect class="drift-zone drift-zone-breach" x="' + pad.left + '" y="' + pad.top + '" width="' + plotW + '" height="' + (parseFloat(breachY) - pad.top).toFixed(2) + '"/>' +
        '<rect class="drift-zone drift-zone-watch" x="' + pad.left + '" y="' + breachY + '" width="' + plotW + '" height="' + (parseFloat(watchY) - parseFloat(breachY)).toFixed(2) + '"/>' +
        '<rect class="drift-zone drift-zone-normal" x="' + pad.left + '" y="' + watchY + '" width="' + plotW + '" height="' + ((pad.top + plotH) - parseFloat(watchY)).toFixed(2) + '"/>' +
        grid +
        '<line class="drift-series-chart-threshold drift-series-chart-breach" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + breachY + '" y2="' + breachY + '"/>' +
        '<line class="drift-series-chart-threshold drift-series-chart-watch" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + watchY + '" y2="' + watchY + '"/>' +
        '<line class="drift-series-chart-baseline" x1="' + pad.left + '" x2="' + (W - pad.right) + '" y1="' + baselineY + '" y2="' + baselineY + '"/>' +
        '<path class="drift-series-chart-area" d="' + areaPath + '"/>' +
        '<path class="drift-series-chart-actual" d="' + actualPath + '"/>' +
        '<circle class="drift-series-chart-anomaly" cx="' + lastX + '" cy="' + lastY + '" r="4"><title>' + _escText(viewModel.score || '0.146') + '</title></circle>' +
        '<text class="drift-series-chart-callout" x="' + (parseFloat(lastX) - 18).toFixed(2) + '" y="' + (parseFloat(lastY) - 18).toFixed(2) + '">' + _escText(viewModel.score || '0.146') + '</text>' +
        labelsY + labelsX + events +
        '<text class="drift-series-zone-label drift-series-zone-label-breach" x="' + (W - 32) + '" y="' + (parseFloat(breachY) - 28).toFixed(2) + '">BREACH</text>' +
        '<text class="drift-series-zone-label drift-series-zone-label-watch" x="' + (W - 32) + '" y="' + (parseFloat(watchY) - 12).toFixed(2) + '">WATCH</text>' +
        '<text class="drift-series-zone-label drift-series-zone-label-normal" x="' + (W - 32) + '" y="' + (pad.top + plotH - 32) + '">NORMAL</text>' +
      '</svg>';
  }

  function clear(mount) {
    if (mount) mount.innerHTML = '';
  }

  window.MIDASExplorerDriftSeriesChart = {
    render: render,
    clear: clear
  };
})();
