// /explorer/assets/js/drift-heatmap.js — Drift-2b
//
// Level 1 Drift Overview heatmap. Renders a matrix of governed
// entities (rows) × V1 drift types (columns) showing the latest
// rolled-up detector status for each (entity, drift_type) cell.
//
// Data source: Option A — frontend-known synthetic DriftDefinition IDs
// produced by the Drift-2a synthetic seed (internal/bootstrap/
// synthetic_drift.go). Drift-1d intentionally does not expose a
// list-all DriftDefinitions endpoint, and Drift-2b is an Explorer-only
// tranche that must not add one. The constant below pins the ten
// stable IDs the synthetic seed creates. Replace with a backend list
// or entity-index endpoint when production drift volume exists.
//
// Read endpoints used (all Drift-1d, all GET):
//   /v1/drift/definitions/{id}
//   /v1/drift/definitions/{id}/series
//   /v1/drift/series/{id}/points?limit=${DRIFT_POINT_FETCH_LIMIT}
//
// Runtime-inert: this module reads only. No POST / PUT / PATCH /
// DELETE. No control-plane lifecycle calls. No drift repository
// mutation. No aggregation, detector, audit-chain, or envelope
// construction code.
//
// Drift-2c shared state: loaded definitions / series / latest-points
// and the built model are exposed on window.MIDASExplorerDrift so the
// Level 2 Drift Workbench (drift-workbench.js) can open without
// refetching definition or series metadata.

(function () {
  'use strict';

  // ── Shared point-fetch limit ───────────────────────────────────────
  // Single source of truth for the page-size used by both the heatmap
  // (latest-point lookup) and the Drift-2c workbench (full series
  // chart). The literal value is acceptable for Drift-2a synthetic
  // data because every series carries 30 daily points; production
  // drift volume will need a backend latest/range/sort endpoint.
  // TODO: replace client-side point loading with backend latest/range/sort
  // support before production-volume drift.
  const DRIFT_POINT_FETCH_LIMIT = 100;

  // ── Demo definition IDs (extracted from internal/bootstrap/synthetic_drift.go) ──
  const DRIFT_DEMO_DEFINITION_IDS = [
    'drift-demo-merchant-fraud-invocation',
    'drift-demo-credit-outcome',
    'drift-demo-id-verify-evidence',
    'drift-demo-fraud-policy',
    'drift-demo-credit-authority',
    'drift-demo-payments-coverage',
    'drift-demo-fraud-system-confidence',
    'drift-demo-evaluator-agent-invocation',
    'drift-demo-fraud-capability-coverage',
    'drift-demo-fraud-grant-authority',
  ];

  // ── V1 column set ──────────────────────────────────────────────────
  // Exactly the nine V1 drift types in display order. V2 drift types
  // (population, data, prediction, performance, concept) are
  // deliberately not represented and must not be added here.
  const DRIFT_TYPE_COLUMNS = [
    { key: 'invocation',   label: 'Invocation' },
    { key: 'outcome',      label: 'Outcome' },
    { key: 'confidence',   label: 'Confidence' },
    { key: 'latency',      label: 'Latency' },
    { key: 'evidence',     label: 'Evidence' },
    { key: 'authority',    label: 'Authority' },
    { key: 'policy',       label: 'Policy' },
    { key: 'coverage',     label: 'Coverage' },
    { key: 'process_path', label: 'Process Path' },
  ];

  // ── Worst-status precedence ────────────────────────────────────────
  // Operator-centric ordering: a breach is the strongest signal; a
  // broken detector is the next worst because monitoring itself has
  // failed; warning signals come next; insufficient_data is "wait /
  // cold-start"; healthy is normal; empty means no definition covers
  // this (entity, drift_type) pair. The five-band detector set
  // deliberately keeps insufficient_data and detector_error apart —
  // there is no single 'unknown' status.
  const WORST_STATUS_ORDER = [
    'breached',
    'unknown_detector_error',
    'warning',
    'unknown_insufficient_data',
    'healthy',
    'empty',
  ];

  function statusRank(status) {
    const idx = WORST_STATUS_ORDER.indexOf(status);
    return idx < 0 ? WORST_STATUS_ORDER.length : idx;
  }

  // ── CSS class mapping ──────────────────────────────────────────────
  // Exactly the six allowed status classes. Generic 'drift-status-
  // unknown' is forbidden; the two unknown variants are spelled out
  // in full so a future refactor cannot collapse them.
  const STATUS_CLASS = {
    healthy:                   'drift-status-healthy',
    warning:                   'drift-status-warning',
    breached:                  'drift-status-breached',
    unknown_insufficient_data: 'drift-status-unknown-insufficient-data',
    unknown_detector_error:    'drift-status-unknown-detector-error',
    empty:                     'drift-status-empty',
  };

  // ── Fetch helpers ──────────────────────────────────────────────────
  // All endpoints are Drift-1d read APIs. Returns { ok, status, json }
  // so callers can branch on transport / parse failures without
  // throwing inside the data-loading pipeline. credentials:
  // 'same-origin' lets the local-IAM session cookie ride along when
  // auth.mode=required; under auth.mode=open the role gate is a
  // no-op.

  function driftFetchJSON(url) {
    return fetch(url, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json' },
    })
      .then(function (resp) {
        if (!resp.ok) {
          return { ok: false, status: resp.status, json: null };
        }
        return resp.json().then(function (j) {
          return { ok: true, status: resp.status, json: j };
        });
      })
      .catch(function (err) {
        return { ok: false, status: 0, json: null, error: (err && err.message) || 'fetch failed' };
      });
  }

  function fetchDriftDefinition(id) {
    return driftFetchJSON('/v1/drift/definitions/' + encodeURIComponent(id));
  }

  function fetchDriftSeriesForDefinition(id) {
    return driftFetchJSON('/v1/drift/definitions/' + encodeURIComponent(id) + '/series');
  }

  // TODO: replace client-side latest-point selection with a backend latest-point
  // or descending-sort endpoint when production drift volume exists.
  function fetchDriftSeriesPoints(seriesID) {
    return driftFetchJSON('/v1/drift/series/' + encodeURIComponent(seriesID)
      + '/points?limit=' + DRIFT_POINT_FETCH_LIMIT);
  }

  // ── Latest-point selection ─────────────────────────────────────────
  // The Drift-1d points endpoint returns points sorted ascending by
  // window_start; the latest is therefore typically the last array
  // entry. Compute the max(window_end) explicitly so the Explorer
  // does not depend on backend ordering — a small defensive cost in
  // exchange for stability if sort order ever changes.
  function selectLatestPointByWindowEnd(points) {
    if (!points || points.length === 0) return null;
    var best = null;
    var bestT = -Infinity;
    for (var i = 0; i < points.length; i++) {
      var p = points[i];
      if (!p || !p.window_end) continue;
      var t = Date.parse(p.window_end);
      if (isNaN(t)) continue;
      if (t > bestT) {
        bestT = t;
        best = p;
      }
    }
    return best;
  }

  // ── Cell derivation ────────────────────────────────────────────────
  // Given the loaded { definitions, seriesByDef, latestPointBySeries }
  // shape, produce a {(entity_key, drift_type) → cell} map plus the
  // ordered row list. Multiple series mapping to the same (entity,
  // drift_type) are reduced via worst-status precedence.

  function entityKey(kind, id) {
    return kind + ':' + id;
  }

  function buildHeatmapModel(loaded) {
    var rowsByKey = {};
    var cellsByKey = {};

    for (var i = 0; i < loaded.definitions.length; i++) {
      var def = loaded.definitions[i];
      if (!def || !def.target) continue;
      var key = entityKey(def.target.kind, def.target.id);
      if (!rowsByKey[key]) {
        rowsByKey[key] = {
          key: key,
          kind: def.target.kind,
          id: def.target.id,
          name: def.target.id, // display-name enrichment deferred to a later tranche
          seriesCount: 0,
          observationCount: 0,
        };
      }
      // Build a metric_id → drift_type lookup for this revision so a
      // series can be classified into its drift-type column without
      // re-fetching the definition per series.
      var metricIDtoDrift = {};
      var metrics = def.metrics || [];
      for (var m = 0; m < metrics.length; m++) {
        metricIDtoDrift[metrics[m].metric_id] = metrics[m].drift_type;
      }
      var seriesList = (loaded.seriesByDef[def.id] || []);
      rowsByKey[key].seriesCount += seriesList.length;
      for (var s = 0; s < seriesList.length; s++) {
        var series = seriesList[s];
        if (!series) continue;
        var driftType = metricIDtoDrift[series.metric_id] || null;
        if (!driftType) continue;
        var latest = loaded.latestPointBySeries[series.id] || null;
        var status = latest ? latest.status : null;
        if (!status) continue;
        var cellKey = key + '||' + driftType;
        var existing = cellsByKey[cellKey];
        var candidate = {
          entityKind: def.target.kind,
          entityID: def.target.id,
          driftType: driftType,
          status: status,
          seriesID: series.id,
          latestWindowEnd: latest.window_end,
          sampleCount: latest.sample_count,
          magnitude: latest.magnitude,
          computationMode: latest.computation_mode,
          backfilled: latest.computation_mode === 'backfilled',
        };
        if (!existing || statusRank(candidate.status) < statusRank(existing.status)) {
          cellsByKey[cellKey] = candidate;
        }
      }
    }

    var rows = Object.keys(rowsByKey).sort().map(function (k) { return rowsByKey[k]; });
    return { rows: rows, cellsByKey: cellsByKey };
  }

  // ── Render helpers ─────────────────────────────────────────────────

  function escapeHTML(s) {
    if (s === null || s === undefined) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function renderColumnHeaders() {
    var html = '<div class="drift-heatmap-cell-header drift-heatmap-corner" aria-hidden="true">Entity</div>';
    for (var i = 0; i < DRIFT_TYPE_COLUMNS.length; i++) {
      var col = DRIFT_TYPE_COLUMNS[i];
      html += '<div class="drift-heatmap-cell-header" data-drift-type="' + escapeHTML(col.key) + '" role="columnheader">'
        + escapeHTML(col.label) + '</div>';
    }
    return html;
  }

  function renderRow(row, cellsByKey) {
    var html = '<div class="drift-heatmap-row" role="row" data-entity-key="' + escapeHTML(row.key) + '">';
    html += '<div class="drift-heatmap-entity" role="rowheader">'
      + '<span class="drift-heatmap-entity-name">' + escapeHTML(row.name) + '</span>'
      + '<span class="drift-heatmap-entity-meta">' + escapeHTML(row.kind) + ' · ' + row.seriesCount + ' series</span>'
      + '</div>';
    for (var i = 0; i < DRIFT_TYPE_COLUMNS.length; i++) {
      var col = DRIFT_TYPE_COLUMNS[i];
      var cellKey = row.key + '||' + col.key;
      var cell = cellsByKey[cellKey];
      var status = cell ? cell.status : 'empty';
      var cls = STATUS_CLASS[status] || STATUS_CLASS.empty;
      var tabIndex = status === 'empty' ? '-1' : '0';
      var title = cell
        ? col.label + ' · ' + status
        : 'No definition for ' + col.label;
      html += '<div class="drift-heatmap-cell ' + cls + '"'
        + ' role="gridcell"'
        + ' tabindex="' + tabIndex + '"'
        + ' data-drift-type="' + escapeHTML(col.key) + '"'
        + ' data-entity-key="' + escapeHTML(row.key) + '"'
        + ' data-status="' + escapeHTML(status) + '"'
        + ' title="' + escapeHTML(title) + '"'
        + ' aria-label="' + escapeHTML(row.name + ' ' + col.label + ' ' + status) + '">'
        + '<span class="drift-heatmap-cell-glyph" aria-hidden="true"></span>'
        + '</div>';
    }
    html += '</div>';
    return html;
  }

  function renderSummaryChips(cellsByKey) {
    var counts = { healthy: 0, warning: 0, breached: 0, unknown_insufficient_data: 0, unknown_detector_error: 0 };
    var keys = Object.keys(cellsByKey);
    for (var i = 0; i < keys.length; i++) {
      var c = cellsByKey[keys[i]];
      if (c && counts.hasOwnProperty(c.status)) counts[c.status]++;
    }
    var chips = [
      { key: 'healthy',                   label: 'Healthy' },
      { key: 'warning',                   label: 'Warning' },
      { key: 'breached',                  label: 'Breached' },
      { key: 'unknown_insufficient_data', label: 'Insufficient data' },
      { key: 'unknown_detector_error',    label: 'Detector error' },
    ];
    var html = '';
    for (var j = 0; j < chips.length; j++) {
      var k = chips[j].key;
      html += '<span class="drift-overview-summary-chip" data-status="' + escapeHTML(k) + '">'
        + escapeHTML(chips[j].label)
        + ' <span class="drift-overview-summary-chip-count">' + counts[k] + '</span>'
        + '</span>';
    }
    return { html: html, counts: counts };
  }

  function renderDetail(detailEl, cell) {
    if (!detailEl) return;
    if (!cell) {
      detailEl.setAttribute('hidden', '');
      detailEl.innerHTML = '';
      return;
    }
    detailEl.removeAttribute('hidden');
    var fields = [
      { k: 'entity kind',     v: cell.entityKind },
      { k: 'entity id',       v: cell.entityID },
      { k: 'drift type',      v: cell.driftType },
      { k: 'status',          v: cell.status },
      { k: 'series id',       v: cell.seriesID },
      { k: 'latest window',   v: cell.latestWindowEnd || '—' },
      { k: 'sample count',    v: cell.sampleCount },
      { k: 'magnitude',       v: cell.magnitude },
      { k: 'computation',     v: cell.computationMode || '—' },
      { k: 'backfilled',      v: cell.backfilled ? 'yes' : 'no' },
    ];
    var rows = '';
    for (var i = 0; i < fields.length; i++) {
      rows += '<div><span class="key">' + escapeHTML(fields[i].k) + '</span>'
        + escapeHTML(String(fields[i].v === undefined || fields[i].v === null ? '—' : fields[i].v))
        + '</div>';
    }
    detailEl.innerHTML =
      '<div class="drift-heatmap-detail-header">'
        + '<span class="drift-heatmap-detail-title">' + escapeHTML(cell.entityID) + ' · ' + escapeHTML(cell.driftType) + '</span>'
        + '<span class="drift-heatmap-detail-meta">' + escapeHTML(cell.status) + '</span>'
      + '</div>'
      + '<div class="drift-heatmap-detail-fields">' + rows + '</div>';
  }

  // ── Data loading orchestration ─────────────────────────────────────

  function loadDriftHeatmapData() {
    var defResults = [];
    var seriesResults = [];

    return Promise.all(DRIFT_DEMO_DEFINITION_IDS.map(fetchDriftDefinition))
      .then(function (results) {
        defResults = results;
        // Detect "Drift repository not configured": Drift-1d returns
        // 501 Not Implemented when the read service is nil. A single
        // 501 across the demo set is enough to conclude the
        // repository is not configured.
        var sawNotImplemented = results.some(function (r) { return r && r.status === 501; });
        if (sawNotImplemented) {
          return { repositoryUnavailable: true };
        }
        var defs = results.filter(function (r) { return r && r.ok && r.json; }).map(function (r) { return r.json; });
        if (defs.length === 0) {
          return { noDefinitions: true };
        }
        return Promise.all(defs.map(function (def) { return fetchDriftSeriesForDefinition(def.id); }))
          .then(function (seriesPayloads) {
            seriesResults = seriesPayloads;
            var seriesByDef = {};
            var allSeries = [];
            for (var i = 0; i < defs.length; i++) {
              var sr = seriesPayloads[i];
              var list = (sr && sr.ok && sr.json && sr.json.drift_series) || [];
              seriesByDef[defs[i].id] = list;
              for (var j = 0; j < list.length; j++) allSeries.push(list[j]);
            }
            if (allSeries.length === 0) {
              return { definitions: defs, seriesByDef: seriesByDef, noSeries: true };
            }
            return Promise.all(allSeries.map(function (s) { return fetchDriftSeriesPoints(s.id); }))
              .then(function (pointPayloads) {
                var latestPointBySeries = {};
                var pointsFound = false;
                var partial = false;
                for (var k = 0; k < allSeries.length; k++) {
                  var pp = pointPayloads[k];
                  if (!pp || !pp.ok) { partial = true; continue; }
                  var pts = (pp.json && pp.json.drift_series_points) || [];
                  var latest = selectLatestPointByWindowEnd(pts);
                  if (latest) {
                    latestPointBySeries[allSeries[k].id] = latest;
                    pointsFound = true;
                  }
                }
                return {
                  definitions: defs,
                  seriesByDef: seriesByDef,
                  latestPointBySeries: latestPointBySeries,
                  noPoints: !pointsFound,
                  partial: partial,
                };
              });
          });
      });
  }

  // ── Module-level cache (Drift-2c) ──────────────────────────────────
  // The most recent successful load result and the model derived from
  // it. The Drift-2c workbench reads these so opening a cell does not
  // re-fetch definitions or series metadata. Cleared if a subsequent
  // load fails. Read via getDriftLoaded() / getDriftModel().
  var lastLoaded = null;
  var lastModel = null;

  function getDriftLoaded() { return lastLoaded; }
  function getDriftModel()  { return lastModel; }

  // ── Top-level render ───────────────────────────────────────────────

  function renderDriftHeatmap(loaded) {
    var view = document.getElementById('services-drift-view');
    if (!view) return;
    var matrixHost = view.querySelector('[data-drift-heatmap]');
    var stateHost = view.querySelector('[data-drift-state]');
    var summaryHost = view.querySelector('[data-drift-summary]');
    var detailHost = view.querySelector('[data-drift-detail]');
    if (!matrixHost || !stateHost) return;

    // Always clear before rendering — the function is also invoked on
    // re-entry to refresh a stale view.
    matrixHost.innerHTML = '';
    stateHost.innerHTML = '';
    stateHost.className = 'drift-overview-state';
    stateHost.setAttribute('hidden', '');
    if (summaryHost) summaryHost.innerHTML = '';
    if (detailHost) {
      detailHost.setAttribute('hidden', '');
      detailHost.innerHTML = '';
    }

    if (!loaded) {
      stateHost.removeAttribute('hidden');
      stateHost.classList.add('is-error');
      stateHost.innerHTML = '<strong>Drift load failed.</strong>'
        + 'Some drift signals could not be loaded.';
      return;
    }

    if (loaded.repositoryUnavailable) {
      stateHost.removeAttribute('hidden');
      stateHost.classList.add('is-error');
      stateHost.innerHTML = '<strong>Drift repository is not configured.</strong>';
      return;
    }
    if (loaded.noDefinitions) {
      stateHost.removeAttribute('hidden');
      stateHost.classList.add('is-empty');
      stateHost.innerHTML = '<strong>No synthetic drift data found. Enable MIDAS_DEV_SEED_SYNTHETIC_DRIFT.</strong>';
      return;
    }
    if (loaded.noSeries) {
      stateHost.removeAttribute('hidden');
      stateHost.classList.add('is-empty');
      stateHost.innerHTML = '<strong>Drift definitions found, but no series are available.</strong>';
      return;
    }
    if (loaded.noPoints) {
      stateHost.removeAttribute('hidden');
      stateHost.classList.add('is-empty');
      stateHost.innerHTML = '<strong>Drift series found, but no points are available.</strong>';
      return;
    }

    var model = buildHeatmapModel(loaded);
    // Drift-2c — cache for the workbench to read without refetching.
    lastLoaded = loaded;
    lastModel = model;

    // Render matrix.
    var html = renderColumnHeaders();
    for (var i = 0; i < model.rows.length; i++) {
      html += renderRow(model.rows[i], model.cellsByKey);
    }
    matrixHost.innerHTML = html;

    // Summary chips.
    if (summaryHost) {
      var summary = renderSummaryChips(model.cellsByKey);
      summaryHost.innerHTML = summary.html;
      // All-healthy summary banner if no warning/breached cells.
      if (summary.counts.warning === 0 && summary.counts.breached === 0
          && summary.counts.unknown_detector_error === 0) {
        // All-empty-cells fourth empty-state: definitions loaded but
        // every resolved cell is empty (Clarification 4). Distinguished
        // here from "no definitions" / "no series" / "no points".
        var anyResolved = Object.keys(model.cellsByKey).length > 0;
        if (!anyResolved) {
          stateHost.removeAttribute('hidden');
          stateHost.classList.add('is-allempty');
          stateHost.innerHTML = '<strong>Drift definitions exist but no series data is available yet.</strong>';
        } else {
          stateHost.removeAttribute('hidden');
          stateHost.classList.add('is-allhealthy');
          stateHost.innerHTML = '<strong>No warning or breached drift signals.</strong>';
        }
      }
    }

    // Partial load notice — additive to any state already shown.
    if (loaded.partial) {
      // If a state strip is already showing, append; otherwise show
      // the partial notice on its own.
      if (stateHost.hasAttribute('hidden')) {
        stateHost.removeAttribute('hidden');
        stateHost.classList.add('is-partial');
        stateHost.innerHTML = '<strong>Partial drift load.</strong>'
          + 'Some drift signals could not be loaded.';
      } else {
        var partialNode = document.createElement('div');
        partialNode.className = 'drift-overview-state is-partial';
        partialNode.innerHTML = '<strong>Partial drift load.</strong>'
          + 'Some drift signals could not be loaded.';
        stateHost.parentNode.insertBefore(partialNode, stateHost.nextSibling);
      }
    }

    // Wire cell selection — hover + focus update the inline detail
    // panel; click / Enter / Space additionally open the Drift-2c
    // Level 2 workbench and apply a persistent .is-selected state.
    var cells = matrixHost.querySelectorAll('.drift-heatmap-cell');
    var selectedCellEl = null;
    function clearSelectedCell() {
      if (selectedCellEl) {
        selectedCellEl.classList.remove('is-selected');
        selectedCellEl.removeAttribute('aria-selected');
      }
      selectedCellEl = null;
    }
    // Drift-2c — exposed so the workbench Close action can deselect
    // the heatmap cell when the workbench is hidden.
    window.MIDASExplorerDrift._clearSelectedHeatmapCell = clearSelectedCell;
    cells.forEach(function (cellEl) {
      var hoverOrFocus = function () {
        var status = cellEl.getAttribute('data-status');
        if (status === 'empty') return;
        var entityKey = cellEl.getAttribute('data-entity-key');
        var driftType = cellEl.getAttribute('data-drift-type');
        var cell = model.cellsByKey[entityKey + '||' + driftType];
        if (detailHost) renderDetail(detailHost, cell);
      };
      var activateCell = function () {
        var status = cellEl.getAttribute('data-status');
        var entityKey = cellEl.getAttribute('data-entity-key');
        var driftType = cellEl.getAttribute('data-drift-type');
        var cell = status === 'empty' ? null : model.cellsByKey[entityKey + '||' + driftType];
        clearSelectedCell();
        cellEl.classList.add('is-selected');
        cellEl.setAttribute('aria-selected', 'true');
        selectedCellEl = cellEl;
        if (window.MIDASExplorerDriftWorkbench
            && typeof window.MIDASExplorerDriftWorkbench.openDriftWorkbench === 'function') {
          window.MIDASExplorerDriftWorkbench.openDriftWorkbench({
            entityKey: entityKey,
            entityKind: cell ? cell.entityKind : null,
            entityID: cell ? cell.entityID : null,
            driftType: driftType,
            cell: cell, // null when the heatmap cell is empty
          });
        }
      };
      cellEl.addEventListener('mouseenter', hoverOrFocus);
      cellEl.addEventListener('focus', hoverOrFocus);
      cellEl.addEventListener('click', activateCell);
      cellEl.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
          e.preventDefault();
          activateCell();
        }
      });
    });
  }

  function loadDriftHeatmap() {
    var view = document.getElementById('services-drift-view');
    if (!view) return Promise.resolve();
    return loadDriftHeatmapData().then(renderDriftHeatmap, function () {
      renderDriftHeatmap(null);
    });
  }

  // Public surface — the inline IIFE in index.html wires the entry
  // button + sub-view transitions and calls window.MIDASExplorerDrift.
  // The Drift-2c workbench module also reads from this namespace.
  var existing = window.MIDASExplorerDrift || {};
  window.MIDASExplorerDrift = Object.assign(existing, {
    DRIFT_DEMO_DEFINITION_IDS: DRIFT_DEMO_DEFINITION_IDS,
    DRIFT_TYPE_COLUMNS: DRIFT_TYPE_COLUMNS,
    WORST_STATUS_ORDER: WORST_STATUS_ORDER,
    STATUS_CLASS: STATUS_CLASS,
    DRIFT_POINT_FETCH_LIMIT: DRIFT_POINT_FETCH_LIMIT,
    statusRank: statusRank,
    selectLatestPointByWindowEnd: selectLatestPointByWindowEnd,
    buildHeatmapModel: buildHeatmapModel,
    loadDriftHeatmap: loadDriftHeatmap,
    renderDriftHeatmap: renderDriftHeatmap,
    driftFetchJSON: driftFetchJSON,
    getDriftLoaded: getDriftLoaded,
    getDriftModel: getDriftModel,
  });
})();
