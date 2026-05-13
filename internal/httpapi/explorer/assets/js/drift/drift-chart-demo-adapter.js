// /explorer/assets/js/drift/drift-chart-demo-adapter.js — D32e-impl-1
//
// Honest demo / synthetic data adapter for the Drift Analytics panel.
// The backend already provides real time-series via
// /v1/drift/series/{id}/points, but that path requires a known
// drift series id linked to the currently-selected business service
// or graph node. Until the Explorer surfaces that mapping for every
// service, this adapter manufactures a deterministic synthetic
// series so the panel renders meaningful content during a
// demonstration.
//
// The adapter MUST:
//   • produce values that are deterministic for a given (key, metric,
//     range) tuple so screenshots / tests are stable
//   • carry the isDemo=true flag so the UI labels the data honestly
//   • never call fetch or pretend to come from a backend
//
// The hash + walk-style generator below is identical in spirit to
// the synthetic generator previously hosted inside the Runtime
// Evidence tray (context-evidence-tray.js); the production path
// will replace this adapter once every service has a resolvable
// drift series id.
//
// Public surface (window.MIDASExplorerDriftChartAdapter):
//
//   fromServiceContext({ serviceId, metric, range })
//   fromGraphNode({ nodeId, metric, range })
//     Return { series: [{ id, label, metric, severity, points:[{t,v}],
//                         baseline:[{t,v}]?, anomalies:[{t,v}]?,
//                         isDemo:true }], isDemo: true }
//
//   isDemoData(result)
//     Type-guard. Returns true when result.isDemo === true.

(function () {
  'use strict';

  function hashKey(seed) {
    var s = String(seed || '');
    var h = 2166136261;
    for (var i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = (h * 16777619) >>> 0;
    }
    return h;
  }

  // pseudoRandom — Mulberry32-style sequence seeded by hash. Each
  // call advances the seed by reference; the next() function returns
  // a value in [0, 1).
  function seqFromSeed(seed) {
    var s = seed >>> 0;
    return function () {
      s = (s + 0x6D2B79F5) >>> 0;
      var t = s;
      t = Math.imul(t ^ (t >>> 15), t | 1);
      t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
      return (((t ^ (t >>> 14)) >>> 0) % 1000000) / 1000000;
    };
  }

  function pointsFor(seed, npoints, baseline, amplitude, drift) {
    var rng = seqFromSeed(seed);
    var out = [];
    var now = Date.now();
    var step = 60 * 60 * 1000; // 1h steps
    var driftSlope = (drift || 0) / Math.max(1, npoints);
    for (var i = npoints - 1; i >= 0; i--) {
      var t = now - i * step;
      var noise = (rng() - 0.5) * amplitude;
      var v = baseline + driftSlope * (npoints - i) + noise;
      out.push({ t: t, v: v });
    }
    return out;
  }

  function rangeToPointCount(range) {
    switch (range) {
      case '24h': return 24;
      case '7d':  return 7 * 24;
      case '30d': return 30 * 8; // hourly thinned
      default:    return 48;
    }
  }

  function _metricSpec(metric) {
    switch (metric) {
      case 'escalation-rate':
        return { label: 'Escalation rate', baseline: 0.08, amplitude: 0.04, drift: 0.05, severity: 'warning' };
      case 'evidence-completeness':
        return { label: 'Evidence completeness', baseline: 0.91, amplitude: 0.03, drift: -0.04, severity: 'info' };
      case 'decision-volume':
        return { label: 'Decision volume', baseline: 420, amplitude: 80, drift: 60, severity: 'info' };
      default:
        return { label: 'Drift signal', baseline: 0.5, amplitude: 0.15, drift: 0.1, severity: 'info' };
    }
  }

  function _buildSeriesForKey(key, range) {
    var metrics = ['escalation-rate', 'evidence-completeness', 'decision-volume'];
    var n = rangeToPointCount(range);
    return metrics.map(function (m, idx) {
      var spec = _metricSpec(m);
      var seed = hashKey(key + ':' + m + ':' + range);
      var pts = pointsFor(seed, n, spec.baseline, spec.amplitude, spec.drift);
      // Baseline line = constant at the metric's intrinsic baseline.
      var baseline = pts.map(function (p) { return { t: p.t, v: spec.baseline }; });
      // Anomaly markers — points that exceed baseline +/- 2 * amplitude.
      var anomalies = pts.filter(function (p) {
        return Math.abs(p.v - spec.baseline) > spec.amplitude * 1.8;
      });
      return {
        id:        'demo-' + m + '-' + (key || 'none'),
        label:     spec.label,
        metric:    m,
        severity:  anomalies.length > 0 ? (idx === 0 ? 'critical' : spec.severity) : 'info',
        points:    pts,
        baseline:  baseline,
        anomalies: anomalies,
        isDemo:    true,
      };
    });
  }

  function fromServiceContext(opts) {
    opts = opts || {};
    var key = opts.serviceId || 'no-service';
    var range = opts.range || '7d';
    return {
      series: _buildSeriesForKey('svc:' + key, range),
      isDemo: true,
      context: { serviceId: opts.serviceId || '', range: range },
    };
  }

  function fromGraphNode(opts) {
    opts = opts || {};
    var key = opts.nodeId || 'no-node';
    var range = opts.range || '7d';
    return {
      series: _buildSeriesForKey('node:' + key, range),
      isDemo: true,
      context: { nodeId: opts.nodeId || '', range: range },
    };
  }

  function isDemoData(result) {
    return !!(result && result.isDemo === true);
  }

  window.MIDASExplorerDriftChartAdapter = {
    fromServiceContext: fromServiceContext,
    fromGraphNode:      fromGraphNode,
    isDemoData:         isDemoData,
    // Exposed for tests; not part of the documented surface.
    _hashKey:           hashKey,
    _rangeToPointCount: rangeToPointCount,
  };
})();
