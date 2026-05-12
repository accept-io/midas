// /explorer/assets/js/drift-workbench.js — Drift-2c
//
// Level 2 Drift Workbench. Opened from a Drift-2b heatmap cell. Shows
// the selected entity × drift_type context, the available series for
// that pair, a magnitude time-series chart, the latest point detail,
// and read-only context for recent observations and annotations.
//
// Layered above drift-heatmap.js: this module reads
// window.MIDASExplorerDrift for shared constants (DRIFT_POINT_FETCH_
// LIMIT, STATUS_CLASS, WORST_STATUS_ORDER, selectLatestPointByWindow
// End, driftFetchJSON, getDriftLoaded, getDriftModel). Constants and
// helpers are NOT redeclared here.
//
// Read endpoints used (all Drift-1d, all GET):
//   /v1/drift/series/{id}/points?limit=${DRIFT_POINT_FETCH_LIMIT}
//   /v1/drift/series/{id}/observations
//   /v1/drift/series/{id}/annotations
//
// Runtime-inert: read-only. No POST / PUT / PATCH / DELETE. No
// control-plane lifecycle calls. No drift repository mutation. No
// aggregation / detection / detector logic. No audit-chain integration.
// No threshold rendering (deferred until chart value semantics are
// metric-specific). No observation triage actions (Level 3).

(function () {
  'use strict';

  // ── Default-selected series ordering ───────────────────────────────
  // Investigation-first: a broken detector must be inspected before
  // interpreting breached values, so unknown_detector_error wins over
  // breached. This order is deliberately distinct from the heatmap's
  // worst-status precedence (which prioritises breached first).
  var WORKBENCH_SELECTION_ORDER = [
    'unknown_detector_error',
    'breached',
    'warning',
    'unknown_insufficient_data',
    'healthy',
    'empty',
  ];

  function workbenchSelectionRank(status) {
    var idx = WORKBENCH_SELECTION_ORDER.indexOf(status);
    return idx < 0 ? WORKBENCH_SELECTION_ORDER.length : idx;
  }

  // ── DOM helpers ────────────────────────────────────────────────────

  function escapeHTML(s) {
    if (s === null || s === undefined) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function statusClassFor(status) {
    var map = (window.MIDASExplorerDrift && window.MIDASExplorerDrift.STATUS_CLASS) || {};
    return map[status] || 'drift-status-empty';
  }

  // Workbench-scope status modifier for chart points / list rows.
  // Parallel to STATUS_CLASS but namespaced under .drift-workbench-
  // point. Backfill is NEVER folded into status — see addBackfillMarker.
  var WORKBENCH_POINT_STATUS_CLASS = {
    healthy:                   'status-healthy',
    warning:                   'status-warning',
    breached:                  'status-breached',
    unknown_insufficient_data: 'status-unknown-insufficient-data',
    unknown_detector_error:    'status-unknown-detector-error',
  };

  function workbenchPointStatusClass(status) {
    return WORKBENCH_POINT_STATUS_CLASS[status] || '';
  }

  // ── Series enumeration for a (entity, drift_type) cell ─────────────
  // The heatmap reduces multiple series to one cell via worst-status
  // precedence; the workbench needs ALL series matching the cell.
  // Walks the cached loaded.definitions + seriesByDef without re-
  // fetching anything from the network.
  function enumerateSeriesForCell(cellContext) {
    var loaded = window.MIDASExplorerDrift && window.MIDASExplorerDrift.getDriftLoaded
      ? window.MIDASExplorerDrift.getDriftLoaded()
      : null;
    if (!loaded || !loaded.definitions) return [];
    var out = [];
    for (var i = 0; i < loaded.definitions.length; i++) {
      var def = loaded.definitions[i];
      if (!def || !def.target) continue;
      if (def.target.kind !== cellContext.entityKind) continue;
      if (def.target.id !== cellContext.entityID) continue;
      var metricIDtoDrift = {};
      var metrics = def.metrics || [];
      for (var m = 0; m < metrics.length; m++) {
        metricIDtoDrift[metrics[m].metric_id] = metrics[m].drift_type;
      }
      var seriesList = (loaded.seriesByDef && loaded.seriesByDef[def.id]) || [];
      for (var s = 0; s < seriesList.length; s++) {
        var series = seriesList[s];
        if (!series) continue;
        var driftType = metricIDtoDrift[series.metric_id] || null;
        if (driftType !== cellContext.driftType) continue;
        var latest = (loaded.latestPointBySeries && loaded.latestPointBySeries[series.id]) || null;
        out.push({
          definition: def,
          series: series,
          metricID: series.metric_id,
          driftType: driftType,
          cadence: series.cadence,
          latestStatus: latest ? latest.status : null,
          latestMagnitude: latest ? latest.magnitude : null,
          latestSampleCount: latest ? latest.sample_count : null,
          latestComputationMode: latest ? latest.computation_mode : null,
          latestBackfilled: latest ? latest.computation_mode === 'backfilled' : false,
        });
      }
    }
    return out;
  }

  function pickDefaultSeries(seriesEntries) {
    if (!seriesEntries || seriesEntries.length === 0) return null;
    if (seriesEntries.length === 1) return seriesEntries[0];
    var best = seriesEntries[0];
    for (var i = 1; i < seriesEntries.length; i++) {
      var bestRank = workbenchSelectionRank(best.latestStatus || 'empty');
      var candRank = workbenchSelectionRank(seriesEntries[i].latestStatus || 'empty');
      if (candRank < bestRank) best = seriesEntries[i];
    }
    return best;
  }

  // ── Fetch helpers ──────────────────────────────────────────────────
  // Reuse the shared driftFetchJSON helper exposed by drift-heatmap.js
  // so a future tranche can swap fetch / auth headers in one place.

  function fetch(url) {
    return window.MIDASExplorerDrift.driftFetchJSON(url);
  }

  function fetchSeriesPoints(seriesID) {
    var limit = window.MIDASExplorerDrift.DRIFT_POINT_FETCH_LIMIT;
    return fetch('/v1/drift/series/' + encodeURIComponent(seriesID)
      + '/points?limit=' + limit);
  }

  function fetchSeriesObservations(seriesID) {
    return fetch('/v1/drift/series/' + encodeURIComponent(seriesID) + '/observations');
  }

  function fetchSeriesAnnotations(seriesID) {
    return fetch('/v1/drift/series/' + encodeURIComponent(seriesID) + '/annotations');
  }

  // ── Workbench state ────────────────────────────────────────────────

  var state = {
    cellContext: null,
    seriesEntries: [],
    activeSeries: null,
    points: [],
    observations: [],
    annotations: [],
    loadErrors: { points: false, observations: false, annotations: false },
    repositoryUnavailable: false,
  };

  // ── Renderers ──────────────────────────────────────────────────────

  function renderHeader(host) {
    if (!host) return;
    if (!state.cellContext) {
      host.innerHTML =
        '<div class="drift-workbench-empty">Select a drift cell to inspect the time series.</div>';
      return;
    }
    var ctx = state.cellContext;
    var status = (state.activeSeries && state.activeSeries.latestStatus) || (ctx.cell ? ctx.cell.status : null);
    var statusClass = status ? statusClassFor(status) : '';
    var metricID = state.activeSeries ? state.activeSeries.metricID : '—';
    var windowSummary = '—';
    if (state.points && state.points.length > 0) {
      var first = state.points[0];
      var last = state.points[state.points.length - 1];
      windowSummary = (first.window_start || '—') + ' → ' + (last.window_end || '—');
    }
    host.innerHTML =
      '<div class="drift-workbench-header-titles">'
        + '<button type="button" class="drift-workbench-close-btn" data-drift-workbench-close'
          + ' aria-label="Close drift workbench">← Close workbench</button>'
        + '<div class="drift-workbench-title">'
          + escapeHTML(ctx.entityID || '—')
          + ' · '
          + escapeHTML(ctx.driftType)
        + '</div>'
        + '<div class="drift-workbench-subtitle">'
          + escapeHTML(ctx.entityKind || '—')
          + ' · metric: ' + escapeHTML(metricID)
          + ' · window: ' + escapeHTML(windowSummary)
        + '</div>'
      + '</div>'
      + (status
          ? '<span class="drift-workbench-header-status ' + statusClass + '">'
              + escapeHTML(status) + '</span>'
          : '')
      + '<div class="drift-synthetic-badge" role="note">'
        + '<span class="drift-synthetic-badge-dot" aria-hidden="true"></span>'
        + '<span>Synthetic demo signal. Not calculated from runtime aggregation.</span>'
      + '</div>';
  }

  function renderSeriesRail(host) {
    if (!host) return;
    if (!state.seriesEntries || state.seriesEntries.length === 0) {
      host.innerHTML = '<div class="drift-workbench-empty">No series available for this cell.</div>';
      return;
    }
    var html = '<div class="drift-workbench-series-rail-title">Series</div>';
    for (var i = 0; i < state.seriesEntries.length; i++) {
      var entry = state.seriesEntries[i];
      var isActive = state.activeSeries && state.activeSeries.series.id === entry.series.id;
      var status = entry.latestStatus || 'empty';
      var rowCls = 'drift-workbench-series-row ' + statusClassFor(status)
        + (isActive ? ' is-active' : '');
      html += '<button type="button" class="' + rowCls + '"'
        + ' role="tab"'
        + ' aria-selected="' + (isActive ? 'true' : 'false') + '"'
        + ' data-drift-workbench-series-id="' + escapeHTML(entry.series.id) + '">'
        + '<span class="drift-workbench-series-row-metric">' + escapeHTML(entry.metricID) + '</span>'
        + '<span class="drift-workbench-series-row-meta">'
          + escapeHTML(entry.cadence || '—') + ' · '
          + escapeHTML(status) + ' · '
          + 'mag ' + escapeHTML(entry.latestMagnitude !== null && entry.latestMagnitude !== undefined ? Number(entry.latestMagnitude).toFixed(3) : '—')
        + '</span>'
        + '<span class="drift-workbench-series-row-meta">'
          + 'samples ' + escapeHTML(entry.latestSampleCount !== null && entry.latestSampleCount !== undefined ? entry.latestSampleCount : '—')
          + ' · ' + escapeHTML(entry.latestComputationMode || '—')
        + '</span>'
        + (entry.latestBackfilled
            ? '<span class="drift-workbench-backfill-marker" aria-label="Latest point is backfilled">backfilled</span>'
            : '')
      + '</button>';
    }
    host.innerHTML = html;
  }

  // ── Chart (vanilla SVG) ────────────────────────────────────────────
  // Plots magnitude over time. No external charting library. Status
  // markers are coloured by the latest detector status of each point;
  // backfilled points additionally carry the .is-backfilled modifier
  // — backfill is rendered as a separate visual cue (dashed outer
  // ring), never as a status colour. No threshold lines are drawn —
  // every metric expresses thresholds in its own units while the
  // chart axis is normalised magnitude across heterogeneous drift
  // types. Adding threshold overlays here would imply precision the
  // model does not yet support.
  // TODO: add detector-aware threshold overlays when chart value
  // semantics are metric-specific.

  function renderChart(host) {
    if (!host) return;
    if (!state.activeSeries) {
      host.innerHTML = '<div class="drift-workbench-empty">No series selected.</div>';
      return;
    }
    if (!state.points || state.points.length === 0) {
      host.innerHTML = '<div class="drift-workbench-empty">This drift series has no points.</div>';
      return;
    }

    // Sort points by window_end ascending for plotting; the latest
    // point for the latest-marker is selected via the shared helper.
    var pts = state.points.slice().sort(function (a, b) {
      var ta = Date.parse(a.window_end || '') || 0;
      var tb = Date.parse(b.window_end || '') || 0;
      return ta - tb;
    });
    var latest = window.MIDASExplorerDrift.selectLatestPointByWindowEnd(pts);

    var width = 600;
    var height = 220;
    var padding = { top: 16, right: 16, bottom: 32, left: 56 };
    var plotW = width - padding.left - padding.right;
    var plotH = height - padding.top - padding.bottom;

    // Compute magnitude domain [0, max] with a small head-room.
    var maxMag = 0;
    for (var i = 0; i < pts.length; i++) {
      var m = Number(pts[i].magnitude);
      if (!isNaN(m) && m > maxMag) maxMag = m;
    }
    if (maxMag <= 0) maxMag = 1;
    var magCeiling = maxMag * 1.1;

    var firstT = Date.parse(pts[0].window_end) || 0;
    var lastT = Date.parse(pts[pts.length - 1].window_end) || (firstT + 1);
    var span = Math.max(lastT - firstT, 1);

    function xAt(p) {
      var t = Date.parse(p.window_end) || firstT;
      return padding.left + ((t - firstT) / span) * plotW;
    }
    function yAt(p) {
      var m = Number(p.magnitude);
      if (isNaN(m)) m = 0;
      return padding.top + plotH - (m / magCeiling) * plotH;
    }

    // Build line path (only over points with classifiable status —
    // unknown_insufficient_data and unknown_detector_error break the
    // line so the chart does not imply continuity through a
    // detector failure).
    var pathSegments = [];
    var segment = '';
    for (var i = 0; i < pts.length; i++) {
      var p = pts[i];
      if (p.status === 'unknown_insufficient_data' || p.status === 'unknown_detector_error') {
        if (segment) pathSegments.push(segment);
        segment = '';
        continue;
      }
      var x = xAt(p);
      var y = yAt(p);
      if (!segment) {
        segment = 'M' + x.toFixed(2) + ',' + y.toFixed(2);
      } else {
        segment += ' L' + x.toFixed(2) + ',' + y.toFixed(2);
      }
    }
    if (segment) pathSegments.push(segment);

    var pointEls = '';
    for (var i = 0; i < pts.length; i++) {
      var p = pts[i];
      var statusCls = workbenchPointStatusClass(p.status);
      var isBackfilled = p.computation_mode === 'backfilled';
      var isLatest = latest && p.id === latest.id;
      var classes = ['drift-workbench-point'];
      if (statusCls) classes.push(statusCls);
      if (isBackfilled) classes.push('is-backfilled');
      if (isLatest) classes.push('is-latest');
      pointEls += '<circle class="' + classes.join(' ') + '"'
        + ' cx="' + xAt(p).toFixed(2) + '"'
        + ' cy="' + yAt(p).toFixed(2) + '"'
        + ' r="' + (isLatest ? 5 : 4) + '"'
        + ' data-status="' + escapeHTML(p.status || '') + '"'
        + ' data-backfilled="' + (isBackfilled ? 'true' : 'false') + '"'
        + (isBackfilled
            ? ' aria-label="Backfilled point ' + escapeHTML(p.window_end || '') + '"'
            : '')
        + '><title>' + escapeHTML('window_end ' + (p.window_end || '—')
            + ' · status ' + (p.status || '—')
            + ' · magnitude ' + (p.magnitude !== undefined ? p.magnitude : '—')
            + (isBackfilled ? ' · backfilled' : ''))
        + '</title></circle>';
      if (isBackfilled) {
        // Distinct outer ring marks backfill orthogonally to status.
        pointEls += '<circle class="drift-workbench-backfill-marker"'
          + ' cx="' + xAt(p).toFixed(2) + '"'
          + ' cy="' + yAt(p).toFixed(2) + '"'
          + ' r="8" aria-hidden="true"></circle>';
      }
    }

    // Y-axis ticks (0, mid, ceiling) — magnitude only. Y-axis label
    // is the literal string "Magnitude" so a future units pass can
    // grep for it cleanly.
    var yMid = magCeiling / 2;
    var axisHTML =
      '<line class="drift-workbench-axis" x1="' + padding.left + '" y1="' + padding.top
        + '" x2="' + padding.left + '" y2="' + (padding.top + plotH) + '"></line>'
      + '<line class="drift-workbench-axis" x1="' + padding.left + '" y1="' + (padding.top + plotH)
        + '" x2="' + (padding.left + plotW) + '" y2="' + (padding.top + plotH) + '"></line>'
      + '<text class="drift-workbench-axis-label" x="' + padding.left + '" y="' + (padding.top - 4)
        + '" text-anchor="start">' + escapeHTML(magCeiling.toFixed(2)) + '</text>'
      + '<text class="drift-workbench-axis-label" x="' + padding.left + '" y="' + (padding.top + plotH / 2)
        + '" text-anchor="end" dx="-4">' + escapeHTML(yMid.toFixed(2)) + '</text>'
      + '<text class="drift-workbench-axis-label" x="' + padding.left + '" y="' + (padding.top + plotH + 4)
        + '" text-anchor="end" dx="-4" dy="10">0.00</text>'
      + '<text class="drift-workbench-axis-title" x="' + (padding.left - 40)
        + '" y="' + (padding.top + plotH / 2)
        + '" text-anchor="middle" transform="rotate(-90 ' + (padding.left - 40)
        + ' ' + (padding.top + plotH / 2) + ')">Magnitude</text>'
      + '<text class="drift-workbench-axis-label" x="' + padding.left + '" y="' + (padding.top + plotH + 18)
        + '">' + escapeHTML((pts[0].window_end || '').slice(0, 10)) + '</text>'
      + '<text class="drift-workbench-axis-label" x="' + (padding.left + plotW) + '" y="' + (padding.top + plotH + 18)
        + '" text-anchor="end">' + escapeHTML((pts[pts.length - 1].window_end || '').slice(0, 10)) + '</text>';

    var lineHTML = '';
    for (var s = 0; s < pathSegments.length; s++) {
      lineHTML += '<path class="drift-workbench-line" d="' + pathSegments[s] + '" fill="none"></path>';
    }

    host.innerHTML =
      '<svg class="drift-workbench-chart" viewBox="0 0 ' + width + ' ' + height
        + '" preserveAspectRatio="none" role="img" aria-label="Drift magnitude time-series">'
        + axisHTML
        + lineHTML
        + pointEls
      + '</svg>'
      + '<div class="drift-workbench-chart-legend" aria-hidden="false">'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch status-healthy"></span>healthy</span>'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch status-warning"></span>warning</span>'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch status-breached"></span>breached</span>'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch status-unknown-insufficient-data"></span>insufficient data</span>'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch status-unknown-detector-error"></span>detector error</span>'
        + '<span class="drift-workbench-chart-legend-item"><span class="drift-workbench-legend-swatch is-backfilled"></span>backfilled</span>'
      + '</div>';
  }

  function renderDetail(host) {
    if (!host) return;
    if (!state.activeSeries) { host.innerHTML = ''; return; }
    var pts = state.points || [];
    if (pts.length === 0) {
      host.innerHTML = '<div class="drift-workbench-empty">This drift series has no points.</div>';
      return;
    }
    var latest = window.MIDASExplorerDrift.selectLatestPointByWindowEnd(pts) || pts[pts.length - 1];
    var fields = [
      { k: 'window_start',          v: latest.window_start || '—' },
      { k: 'window_end',            v: latest.window_end || '—' },
      { k: 'sample_count',          v: latest.sample_count !== undefined ? latest.sample_count : '—' },
      { k: 'magnitude',             v: latest.magnitude !== undefined ? latest.magnitude : '—' },
      { k: 'status',                v: latest.status || '—' },
      { k: 'computation_mode',      v: latest.computation_mode || '—' },
      { k: 'computed_at',           v: latest.computed_at || '—' },
      { k: 'source_window_complete', v: latest.source_window_complete === undefined ? '—' : (latest.source_window_complete ? 'yes' : 'no') },
      { k: 'baseline_window_id',    v: latest.baseline_window_id || '—' },
    ];
    if (latest.computation_mode === 'backfilled' && latest.backfill_run_id) {
      fields.push({ k: 'backfill_run_id', v: latest.backfill_run_id });
    }
    var rows = '';
    for (var i = 0; i < fields.length; i++) {
      rows += '<div><span class="key">' + escapeHTML(fields[i].k) + '</span>'
        + escapeHTML(String(fields[i].v === undefined || fields[i].v === null ? '—' : fields[i].v))
        + '</div>';
    }
    var statusCls = statusClassFor(latest.status);
    host.innerHTML =
      '<div class="drift-workbench-detail-header">'
        + '<span class="drift-workbench-detail-title">Latest point</span>'
        + '<span class="drift-workbench-detail-status ' + statusCls + '">' + escapeHTML(latest.status || '—') + '</span>'
        + (latest.computation_mode === 'backfilled'
            ? '<span class="drift-workbench-backfill-marker">backfilled</span>'
            : '')
      + '</div>'
      + '<div class="drift-workbench-detail-fields">' + rows + '</div>';
  }

  function renderObservations(host) {
    if (!host) return;
    // Drift-2d preserves the static inspector shell at the bottom of
    // this host. The inspector module reads/writes that element by id;
    // re-rendering the observation list must keep the shell intact.
    var inspectorShell = host.querySelector('#drift-observation-inspector');
    if (state.loadErrors.observations) {
      host.innerHTML =
        '<div class="drift-workbench-section-title">Observations</div>'
        + '<div class="drift-workbench-empty">Observations could not be loaded.</div>';
      if (inspectorShell) host.appendChild(inspectorShell);
      return;
    }
    var obs = state.observations || [];
    if (obs.length === 0) {
      host.innerHTML =
        '<div class="drift-workbench-section-title">Observations</div>'
        + '<div class="drift-workbench-empty">No observations on this series.</div>';
      if (inspectorShell) host.appendChild(inspectorShell);
      return;
    }
    // Sort newest detected_at first; cap displayed rows so the panel
    // stays compact. Each row is a button so it is clickable and
    // keyboard-activatable for Drift-2d inspector entry.
    var sorted = obs.slice().sort(function (a, b) {
      return (Date.parse(b.detected_at || '') || 0) - (Date.parse(a.detected_at || '') || 0);
    });
    var rows = '';
    for (var i = 0; i < Math.min(sorted.length, 8); i++) {
      var o = sorted[i];
      var statusCls = statusClassFor(o.detector_status);
      rows += '<li>'
        + '<button type="button"'
          + ' class="drift-workbench-observation-row drift-workbench-observation-item ' + statusCls + '"'
          + ' data-drift-observation-id="' + escapeHTML(o.id || '') + '"'
          + ' aria-label="Inspect observation ' + escapeHTML(o.id || '') + '">'
          + '<div class="drift-workbench-observation-meta">'
            + '<span class="drift-workbench-observation-id">' + escapeHTML(o.id || '—') + '</span>'
            + '<span class="drift-workbench-observation-status">' + escapeHTML(o.detector_status || '—') + '</span>'
            + '<span class="drift-workbench-observation-operator">operator: ' + escapeHTML(o.operator_status || '—') + '</span>'
            + (o.backfilled
                ? '<span class="drift-workbench-backfill-marker">Backfilled observation</span>'
                : '')
          + '</div>'
          + '<div class="drift-workbench-observation-time">' + escapeHTML(o.detected_at || '—') + '</div>'
        + '</button>'
      + '</li>';
    }
    host.innerHTML =
      '<div class="drift-workbench-section-title">Observations</div>'
      + '<ul class="drift-workbench-observation-list">' + rows + '</ul>';
    if (inspectorShell) host.appendChild(inspectorShell);
  }

  function renderAnnotations(host) {
    if (!host) return;
    if (state.loadErrors.annotations) {
      host.innerHTML =
        '<div class="drift-workbench-section-title">Annotations</div>'
        + '<div class="drift-workbench-empty">Annotations could not be loaded.</div>';
      return;
    }
    var ann = state.annotations || [];
    if (ann.length === 0) {
      host.innerHTML =
        '<div class="drift-workbench-section-title">Annotations</div>'
        + '<div class="drift-workbench-empty">No annotations on this series.</div>';
      return;
    }
    var sorted = ann.slice().sort(function (a, b) {
      return (Date.parse(b.created_at || '') || 0) - (Date.parse(a.created_at || '') || 0);
    });
    var rows = '';
    for (var i = 0; i < sorted.length; i++) {
      var a = sorted[i];
      rows += '<li class="drift-workbench-annotation-item">'
        + '<div class="drift-workbench-annotation-meta">'
          + '<span class="drift-workbench-annotation-type">' + escapeHTML(a.annotation_type || '—') + '</span>'
          + '<span class="drift-workbench-annotation-status">' + escapeHTML(a.status || '—') + '</span>'
          + '<span class="drift-workbench-annotation-time">' + escapeHTML(a.created_at || '—') + '</span>'
        + '</div>'
        + '<div class="drift-workbench-annotation-body">' + escapeHTML(a.body || '') + '</div>'
      + '</li>';
    }
    host.innerHTML =
      '<div class="drift-workbench-section-title">Annotations</div>'
      + '<ul class="drift-workbench-annotation-list">' + rows + '</ul>';
  }

  // ── Top-level render ───────────────────────────────────────────────

  function getPanels() {
    var root = document.getElementById('services-drift-workbench');
    if (!root) return null;
    return {
      root: root,
      header: root.querySelector('[data-drift-workbench-header]'),
      seriesRail: root.querySelector('[data-drift-workbench-series-rail]'),
      chart: root.querySelector('[data-drift-workbench-chart]'),
      detail: root.querySelector('[data-drift-workbench-detail]'),
      observations: root.querySelector('[data-drift-workbench-observations]'),
      annotations: root.querySelector('[data-drift-workbench-annotations]'),
      stateHost: root.querySelector('[data-drift-workbench-state]'),
    };
  }

  function showWorkbench() {
    var panels = getPanels();
    if (!panels) return;
    panels.root.removeAttribute('hidden');
  }
  function hideWorkbench() {
    var panels = getPanels();
    if (!panels) return;
    panels.root.setAttribute('hidden', '');
  }

  function renderAll() {
    var panels = getPanels();
    if (!panels) return;

    // Repository-unavailable path — skip everything else and surface a
    // single state strip.
    if (panels.stateHost) {
      panels.stateHost.innerHTML = '';
      panels.stateHost.setAttribute('hidden', '');
    }
    if (state.repositoryUnavailable && panels.stateHost) {
      panels.stateHost.removeAttribute('hidden');
      panels.stateHost.innerHTML = '<strong>Drift repository is not configured.</strong>';
      return;
    }
    // Empty cell — explicit message; skip data fetches in caller.
    if (state.cellContext && state.cellContext.cell === null && panels.stateHost) {
      panels.stateHost.removeAttribute('hidden');
      panels.stateHost.innerHTML = 'No drift definition exists for this entity and drift type.';
    }
    renderHeader(panels.header);
    renderSeriesRail(panels.seriesRail);
    renderChart(panels.chart);
    renderDetail(panels.detail);
    renderObservations(panels.observations);
    renderAnnotations(panels.annotations);
  }

  // ── Workbench load pipeline ────────────────────────────────────────

  function loadActiveSeries() {
    if (!state.activeSeries) {
      state.points = [];
      state.observations = [];
      state.annotations = [];
      state.loadErrors = { points: false, observations: false, annotations: false };
      return Promise.resolve();
    }
    var seriesID = state.activeSeries.series.id;
    return Promise.all([
      fetchSeriesPoints(seriesID),
      fetchSeriesObservations(seriesID),
      fetchSeriesAnnotations(seriesID),
    ]).then(function (results) {
      var pointsRes = results[0];
      var obsRes = results[1];
      var annRes = results[2];
      // 501 Not Implemented on any drift route is interpreted as
      // "Drift repository is not configured" — the heatmap surfaces the
      // same diagnosis so the workbench mirrors it for consistency.
      if ((pointsRes && pointsRes.status === 501)
          || (obsRes && obsRes.status === 501)
          || (annRes && annRes.status === 501)) {
        state.repositoryUnavailable = true;
        return;
      }
      state.repositoryUnavailable = false;
      state.points = (pointsRes && pointsRes.ok && pointsRes.json && pointsRes.json.drift_series_points) || [];
      state.loadErrors.points = !(pointsRes && pointsRes.ok);
      state.observations = (obsRes && obsRes.ok && obsRes.json && obsRes.json.drift_observations) || [];
      state.loadErrors.observations = !(obsRes && obsRes.ok);
      state.annotations = (annRes && annRes.ok && annRes.json && annRes.json.drift_annotations) || [];
      state.loadErrors.annotations = !(annRes && annRes.ok);
    }, function () {
      state.loadErrors = { points: true, observations: true, annotations: true };
    });
  }

  // Drift-2d — track the selected observation row inside the
  // workbench so the inspector entry / close cycle can apply and
  // remove the .is-selected modifier without re-rendering the whole
  // observation list. This state is module-local; the inspector
  // module reads/writes selection-context only via the public hooks
  // below.
  var selectedObservationRowEl = null;
  function setSelectedObservationRow(rowEl) {
    if (selectedObservationRowEl) {
      selectedObservationRowEl.classList.remove('is-selected');
      selectedObservationRowEl.removeAttribute('aria-selected');
    }
    selectedObservationRowEl = rowEl || null;
    if (selectedObservationRowEl) {
      selectedObservationRowEl.classList.add('is-selected');
      selectedObservationRowEl.setAttribute('aria-selected', 'true');
    }
  }
  function clearSelectedObservation() {
    setSelectedObservationRow(null);
    if (window.MIDASExplorerDriftObservationInspector
        && typeof window.MIDASExplorerDriftObservationInspector.closeObservationInspector === 'function') {
      window.MIDASExplorerDriftObservationInspector.closeObservationInspector();
    }
  }

  function setActiveSeriesByID(seriesID) {
    for (var i = 0; i < state.seriesEntries.length; i++) {
      if (state.seriesEntries[i].series.id === seriesID) {
        state.activeSeries = state.seriesEntries[i];
        // Drift-2d — switching series clears any selected observation
        // and closes the inspector before the new series's data loads.
        clearSelectedObservation();
        loadActiveSeries().then(renderAll);
        renderAll(); // immediate visual feedback on rail selection
        return;
      }
    }
  }

  // ── Public entry point ─────────────────────────────────────────────

  function openDriftWorkbench(cellContext) {
    showWorkbench();
    state.cellContext = cellContext;
    state.repositoryUnavailable = false;
    state.loadErrors = { points: false, observations: false, annotations: false };

    // Empty heatmap cell: still show the workbench header with the
    // state strip so the operator gets the explicit diagnosis.
    if (!cellContext || !cellContext.cell) {
      state.seriesEntries = [];
      state.activeSeries = null;
      state.points = [];
      state.observations = [];
      state.annotations = [];
      renderAll();
      return;
    }

    state.seriesEntries = enumerateSeriesForCell(cellContext);
    state.activeSeries = pickDefaultSeries(state.seriesEntries);
    renderAll();
    loadActiveSeries().then(renderAll);
  }

  function closeDriftWorkbench() {
    hideWorkbench();
    if (window.MIDASExplorerDrift
        && typeof window.MIDASExplorerDrift._clearSelectedHeatmapCell === 'function') {
      window.MIDASExplorerDrift._clearSelectedHeatmapCell();
    }
    // Drift-2d — closing the workbench clears any selected observation
    // and closes the inspector so the next workbench open starts clean.
    clearSelectedObservation();
    state.cellContext = null;
    state.activeSeries = null;
    state.seriesEntries = [];
    state.points = [];
    state.observations = [];
    state.annotations = [];
  }

  // ── Wire delegated events on the workbench root ────────────────────

  function activateObservationRow(rowEl) {
    if (!rowEl) return;
    var obsID = rowEl.getAttribute('data-drift-observation-id');
    if (!obsID) return;
    setSelectedObservationRow(rowEl);
    if (window.MIDASExplorerDriftObservationInspector
        && typeof window.MIDASExplorerDriftObservationInspector.openObservationInspector === 'function') {
      // Pass the cached observation payload too so the inspector can
      // render header content immediately while the full fetch
      // (defensive re-fetch for the canonical shape) is in flight.
      var cached = null;
      for (var i = 0; state.observations && i < state.observations.length; i++) {
        if (state.observations[i] && state.observations[i].id === obsID) {
          cached = state.observations[i];
          break;
        }
      }
      window.MIDASExplorerDriftObservationInspector.openObservationInspector(obsID, cached);
    }
  }

  function wireWorkbenchEvents() {
    var root = document.getElementById('services-drift-workbench');
    if (!root) return;
    root.addEventListener('click', function (e) {
      var target = e.target;
      // Walk up so clicks on inner spans bubble correctly.
      var seriesRow = target && target.closest ? target.closest('[data-drift-workbench-series-id]') : null;
      if (seriesRow) {
        var sid = seriesRow.getAttribute('data-drift-workbench-series-id');
        if (sid) setActiveSeriesByID(sid);
        return;
      }
      var observationRow = target && target.closest ? target.closest('[data-drift-observation-id]') : null;
      if (observationRow) {
        activateObservationRow(observationRow);
        return;
      }
      var closeBtn = target && target.closest ? target.closest('[data-drift-workbench-close]') : null;
      if (closeBtn) {
        closeDriftWorkbench();
      }
    });
    root.addEventListener('keydown', function (e) {
      var target = e.target;
      var seriesRow = target && target.closest ? target.closest('[data-drift-workbench-series-id]') : null;
      if (seriesRow && (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar')) {
        e.preventDefault();
        var sid = seriesRow.getAttribute('data-drift-workbench-series-id');
        if (sid) setActiveSeriesByID(sid);
        return;
      }
      var observationRow = target && target.closest ? target.closest('[data-drift-observation-id]') : null;
      if (observationRow && (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar')) {
        e.preventDefault();
        activateObservationRow(observationRow);
      }
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireWorkbenchEvents);
  } else {
    wireWorkbenchEvents();
  }

  // Public surface — heatmap module calls openDriftWorkbench when a
  // non-empty cell is activated. Drift-2d additionally exposes
  // _clearSelectedObservation so the inspector's Close control can
  // deselect the workbench observation row without poking DOM directly.
  window.MIDASExplorerDriftWorkbench = {
    WORKBENCH_SELECTION_ORDER: WORKBENCH_SELECTION_ORDER,
    workbenchSelectionRank: workbenchSelectionRank,
    enumerateSeriesForCell: enumerateSeriesForCell,
    pickDefaultSeries: pickDefaultSeries,
    openDriftWorkbench: openDriftWorkbench,
    closeDriftWorkbench: closeDriftWorkbench,
    setActiveSeriesByID: setActiveSeriesByID,
    _clearSelectedObservation: clearSelectedObservation,
  };
})();
