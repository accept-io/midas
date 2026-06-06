// /explorer/assets/js/drift/drift-analytics-view-model.js - D32e-tranche-1
//
// Shared Drift Analytics view-model contract. Producers, including the
// demo adapter today and a derive adapter later, build this shape; the
// panel, chart, and contribution rail render it without knowing where
// the data came from.

(function () {
  'use strict';

  var VALUE_SERIES = [
    0.046, 0.044, 0.052, 0.045, 0.056, 0.054, 0.050, 0.060,
    0.052, 0.061, 0.056, 0.058, 0.066, 0.075, 0.062, 0.066,
    0.078, 0.086, 0.080, 0.078, 0.090, 0.074, 0.092, 0.080,
    0.079, 0.082, 0.091, 0.104, 0.101, 0.114, 0.116, 0.106,
    0.105, 0.095, 0.093, 0.096, 0.106, 0.104, 0.103, 0.108,
    0.107, 0.106, 0.117, 0.116, 0.109, 0.117, 0.118, 0.128,
    0.126, 0.116, 0.132, 0.116, 0.124, 0.132, 0.120, 0.128,
    0.122, 0.135, 0.138, 0.136, 0.144, 0.146, 0.146, 0.146
  ];

  var X_TICKS = ['May 30', 'Jun 03', 'Jun 07', 'Jun 11', 'Jun 15', 'Jun 19', 'Jun 23', 'Jun 27', 'Jun 29'];
  var COMPACT_X_TICKS = ['May 30', 'Jun 07', 'Jun 15', 'Jun 23', 'Jun 29'];
  var Y_TICKS = ['0.000', '0.050', '0.100', '0.150', '0.200'];

  function cloneArray(items) {
    return Array.isArray(items) ? items.slice() : [];
  }

  function buildPoints(values) {
    var base = Date.UTC(2026, 4, 30, 12, 0, 0);
    var step = 12 * 60 * 60 * 1000;
    return cloneArray(values).map(function (value, idx) {
      return {
        id: 'point-' + String(idx + 1).padStart(2, '0'),
        t: base + idx * step,
        label: X_TICKS[Math.min(X_TICKS.length - 1, Math.floor(idx / 8))],
        value: value
      };
    });
  }

  function normalise(input) {
    input = input || {};
    var values = Array.isArray(input.values) && input.values.length === 64 ? input.values : VALUE_SERIES;
    var points = Array.isArray(input.points) && input.points.length > 0 ? input.points : buildPoints(values);
    return {
      id: input.id || 'drift-analytics-cards-authority-paths',
      title: input.title || 'Drift Analytics',
      nodeLabel: input.nodeLabel || 'Node bs:bs-cards',
      serviceLabel: input.serviceLabel || 'Cards',
      scoreLabel: input.scoreLabel || 'DRIFT SCORE (composite)',
      compactScoreLabel: input.compactScoreLabel || 'DRIFT SCORE',
      score: input.score || '0.146',
      scoreStatus: input.scoreStatus || 'WATCH',
      scoreSubtitle: input.scoreSubtitle || 'weighted sum of 4 signals',
      breachGap: input.breachGap || '0.006 below breach',
      sourceClassification: input.sourceClassification || 'demo_derived',
      provenanceLabel: input.provenanceLabel || 'Demo evidence',
      provenanceHash: input.provenanceHash || '',
      formula: input.formula || 'score = sum w_i * dev_i; weights: profile v4.2',
      values: cloneArray(values),
      points: points,
      baseline: input.baseline || 0.052,
      baselineBeforeProfileChange: input.baselineBeforeProfileChange || 0.040,
      baselineAfterProfileChange: input.baselineAfterProfileChange || 0.052,
      baselineChangeLabel: input.baselineChangeLabel || 'Jun 07',
      watchThresholdOffset: input.watchThresholdOffset || 0.052,
      breachThresholdOffset: input.breachThresholdOffset || 0.100,
      watchThreshold: input.watchThreshold || 0.104,
      breachThreshold: input.breachThreshold || 0.152,
      yTicks: cloneArray(input.yTicks || Y_TICKS),
      xTicks: cloneArray(input.xTicks || X_TICKS),
      compactXTicks: cloneArray(input.compactXTicks || COMPACT_X_TICKS),
      rangeLabel: input.rangeLabel || 'Last 30 days',
      selectedContributionId: input.selectedContributionId || 'authority-path-divergence',
      contributions: cloneArray(input.contributions || [
        { id: 'authority-path-divergence', label: 'Authority-path divergence', value: '0.071', share: '49%', severity: 'breach', color: 'red' },
        { id: 'escalation-rate-deviation', label: 'Escalation-rate deviation', value: '0.038', share: '26%', severity: 'watch', color: 'amber' },
        { id: 'evidence-completeness-gap', label: 'Evidence-completeness gap', value: '0.022', share: '15%', severity: 'watch', color: 'amber' },
        { id: 'outcome-mix-shift', label: 'Outcome-mix shift', value: '0.015', share: '10%', severity: 'normal', color: 'green' }
      ]),
      events: cloneArray(input.events || [
        { id: 'event-policy-update-jun-01', type: 'policy-update', label: 'Policy update', date: 'Jun 01' },
        { id: 'event-profile-change-jun-07', type: 'profile-change', label: 'Profile change', date: 'Jun 07' },
        { id: 'event-incident-jun-13', type: 'incident', label: 'Incident', date: 'Jun 13' },
        { id: 'event-policy-update-jun-21', type: 'policy-update', label: 'Policy update', date: 'Jun 21' },
        { id: 'event-profile-change-jun-27', type: 'profile-change', label: 'Profile change', date: 'Jun 27' }
      ])
    };
  }

  window.MIDASExplorerDriftAnalyticsViewModel = {
    normalise: normalise,
    buildPoints: buildPoints,
    demoValues64: cloneArray(VALUE_SERIES),
    xTicks: cloneArray(X_TICKS),
    yTicks: cloneArray(Y_TICKS)
  };
})();
