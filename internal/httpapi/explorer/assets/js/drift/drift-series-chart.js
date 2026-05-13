// /explorer/assets/js/drift/drift-series-chart.js — D32e-impl-1
//
// SVG drift time-series chart. Pure renderer: takes a mount element
// + a series object + render options, paints once. No fetch, no
// store, no event listeners except a single hover handler if the
// caller asks for it.
//
// Series shape (matches the demo adapter + future real-data adapter):
//
//   {
//     id, label, metric, severity,
//     points:    [{ t: number, v: number, status? }],
//     baseline?: [{ t: number, v: number }],
//     forecast?: [{ t: number, v: number }],
//     anomalies?: [{ t: number, v: number }],
//     isDemo?: boolean,
//   }
//
// Render options:
//
//   { padding?: number, height?: number, showAnomalies?: boolean,
//     showBaseline?: boolean, ariaLabel?: string }
//
// Public surface (window.MIDASExplorerDriftSeriesChart):
//
//   render(mount, series, options)
//   clear(mount)

(function () {
  'use strict';

  function _formatters() {
    return window.MIDASExplorerDriftChartFormatters || {};
  }

  // _domain — computes time + value extents over actual + baseline +
  // forecast + anomalies so the chart frames everything without
  // clipping.
  function _domain(series) {
    var allPoints = [];
    if (series && Array.isArray(series.points))    allPoints = allPoints.concat(series.points);
    if (series && Array.isArray(series.baseline))  allPoints = allPoints.concat(series.baseline);
    if (series && Array.isArray(series.forecast))  allPoints = allPoints.concat(series.forecast);
    if (series && Array.isArray(series.anomalies)) allPoints = allPoints.concat(series.anomalies);
    if (allPoints.length === 0) return null;
    var minT = Infinity, maxT = -Infinity, minV = Infinity, maxV = -Infinity;
    for (var i = 0; i < allPoints.length; i++) {
      var p = allPoints[i];
      if (typeof p.t === 'number' && isFinite(p.t)) {
        if (p.t < minT) minT = p.t;
        if (p.t > maxT) maxT = p.t;
      }
      if (typeof p.v === 'number' && isFinite(p.v)) {
        if (p.v < minV) minV = p.v;
        if (p.v > maxV) maxV = p.v;
      }
    }
    if (!isFinite(minT) || !isFinite(maxT) || !isFinite(minV) || !isFinite(maxV)) return null;
    if (minT === maxT) maxT = minT + 1;
    if (minV === maxV) { minV -= 0.5; maxV += 0.5; }
    var spanV = maxV - minV;
    return {
      minT: minT, maxT: maxT,
      minV: minV - spanV * 0.08,
      maxV: maxV + spanV * 0.08,
    };
  }

  function _pathFor(points, scaleX, scaleY) {
    if (!Array.isArray(points) || points.length === 0) return '';
    var parts = [];
    for (var i = 0; i < points.length; i++) {
      var p = points[i];
      if (typeof p.t !== 'number' || typeof p.v !== 'number') continue;
      if (!isFinite(p.t) || !isFinite(p.v)) continue;
      var x = scaleX(p.t).toFixed(2);
      var y = scaleY(p.v).toFixed(2);
      parts.push((parts.length === 0 ? 'M' : 'L') + x + ',' + y);
    }
    return parts.join(' ');
  }

  function _renderEmpty(mount, ariaLabel) {
    mount.innerHTML =
      '<div class="drift-series-chart-empty" role="status" aria-live="polite">' +
        'No drift signal selected.' +
      '</div>';
    if (ariaLabel) mount.setAttribute('aria-label', ariaLabel);
  }

  function render(mount, series, options) {
    if (!mount) return;
    options = options || {};
    var ariaLabel = options.ariaLabel || (series && series.label) || 'Drift signal time-series';
    if (!series || !Array.isArray(series.points) || series.points.length === 0) {
      _renderEmpty(mount, ariaLabel);
      return;
    }

    var W = 720;
    var H = options.height || 220;
    var pad = options.padding || { top: 16, right: 16, bottom: 28, left: 44 };
    var plotW = W - pad.left - pad.right;
    var plotH = H - pad.top - pad.bottom;

    var dom = _domain(series);
    if (!dom) { _renderEmpty(mount, ariaLabel); return; }

    var spanT = dom.maxT - dom.minT;
    var spanV = dom.maxV - dom.minV;
    function scaleX(t) { return pad.left + ((t - dom.minT) / spanT) * plotW; }
    function scaleY(v) { return pad.top + plotH - ((v - dom.minV) / spanV) * plotH; }

    var fmt = _formatters();
    var fmtT = (typeof fmt.formatTimestamp === 'function') ? fmt.formatTimestamp : function (t) { return String(t); };
    var fmtV = (typeof fmt.formatValue === 'function') ? fmt.formatValue : function (v) { return String(v); };

    // Y-axis ticks — 4 evenly spaced values across the value range.
    var yTicks = [];
    for (var i = 0; i <= 3; i++) {
      var v = dom.minV + (spanV * i) / 3;
      var y = scaleY(v);
      yTicks.push({ v: v, y: y });
    }
    // X-axis ticks — sample 4 points across the time range.
    var xTicks = [];
    for (var j = 0; j <= 3; j++) {
      var t = dom.minT + (spanT * j) / 3;
      var x = scaleX(t);
      xTicks.push({ t: t, x: x });
    }

    var actualPath   = _pathFor(series.points,   scaleX, scaleY);
    var baselinePath = options.showBaseline !== false ? _pathFor(series.baseline, scaleX, scaleY) : '';
    var forecastPath = _pathFor(series.forecast, scaleX, scaleY);

    var anomalyMarkers = '';
    if (options.showAnomalies !== false && Array.isArray(series.anomalies)) {
      for (var k = 0; k < series.anomalies.length; k++) {
        var a = series.anomalies[k];
        if (typeof a.t !== 'number' || typeof a.v !== 'number') continue;
        anomalyMarkers +=
          '<circle class="drift-series-chart-anomaly"' +
            ' cx="' + scaleX(a.t).toFixed(2) + '"' +
            ' cy="' + scaleY(a.v).toFixed(2) + '"' +
            ' r="4">' +
            '<title>Anomaly · ' + _escAttr(fmtT(a.t)) + ' · ' + _escAttr(fmtV(a.v)) + '</title>' +
          '</circle>';
      }
    }

    var gridY = '';
    for (var gi = 0; gi < yTicks.length; gi++) {
      gridY +=
        '<line class="drift-series-chart-grid"' +
          ' x1="' + pad.left + '"' +
          ' x2="' + (W - pad.right) + '"' +
          ' y1="' + yTicks[gi].y.toFixed(2) + '"' +
          ' y2="' + yTicks[gi].y.toFixed(2) + '" />';
    }

    var yLabels = '';
    for (var yi = 0; yi < yTicks.length; yi++) {
      yLabels +=
        '<text class="drift-series-chart-axis-label"' +
          ' x="' + (pad.left - 6) + '"' +
          ' y="' + (yTicks[yi].y + 3).toFixed(2) + '"' +
          ' text-anchor="end">' + _escText(fmtV(yTicks[yi].v)) + '</text>';
    }
    var xLabels = '';
    for (var xi = 0; xi < xTicks.length; xi++) {
      xLabels +=
        '<text class="drift-series-chart-axis-label"' +
          ' x="' + xTicks[xi].x.toFixed(2) + '"' +
          ' y="' + (H - 8) + '"' +
          ' text-anchor="middle">' + _escText(fmtT(xTicks[xi].t)) + '</text>';
    }

    var sevClass = 'drift-series-chart-severity-' + _escAttr(series.severity || 'info');
    mount.innerHTML =
      '<svg class="drift-series-chart-svg ' + sevClass + '"' +
        ' viewBox="0 0 ' + W + ' ' + H + '"' +
        ' preserveAspectRatio="none"' +
        ' role="img"' +
        ' aria-label="' + _escAttr(ariaLabel) + '">' +
        '<rect class="drift-series-chart-bg"' +
          ' x="' + pad.left + '" y="' + pad.top + '"' +
          ' width="' + plotW + '" height="' + plotH + '"/>' +
        gridY +
        (baselinePath
          ? '<path class="drift-series-chart-baseline" d="' + baselinePath + '"/>'
          : '') +
        (forecastPath
          ? '<path class="drift-series-chart-forecast" d="' + forecastPath + '"/>'
          : '') +
        '<path class="drift-series-chart-actual" d="' + actualPath + '"/>' +
        anomalyMarkers +
        yLabels +
        xLabels +
      '</svg>';
  }

  function clear(mount) {
    if (mount) mount.innerHTML = '';
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

  window.MIDASExplorerDriftSeriesChart = {
    render: render,
    clear:  clear,
  };
})();
