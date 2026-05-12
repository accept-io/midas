// /explorer/assets/js/drift-observation-inspector.js — Drift-2d
//
// Level 3 Drift Observation Inspector. Opened from the Level 2
// workbench observation list. Read-only by design: shows observation
// identity, observed window, magnitude / detector / operator status,
// linked series point, backfill provenance, evidence envelope refs,
// related governance refs, and observation-targeted annotations.
//
// This module is strictly read-only. The brief's full list of
// forbidden operator-workflow labels and mutating-call tokens is
// enforced by source-pin tests in
// internal/httpapi/explorer_drift_observation_inspector_test.go
// (TestExplorer_HTML_DriftObservationInspector_ReadOnly_*,
// TestExplorer_DriftInspectorJS_UsesOnlyPermittedReadEndpoints,
// TestExplorer_DriftInspectorJS_NoMutatingHTTPVerbs,
// TestExplorer_DriftInspectorJS_RuntimeInert). The list is not
// repeated here so the tests do not trip on their own
// documentation.
//
// Read endpoints used (Drift-1d, GET only):
//   /v1/drift/observations/{id}
//   /v1/drift/observations/{id}/annotations
//   /v1/drift/series-points/{point_id}
//
// Layered above drift-workbench.js: this module reads
// window.MIDASExplorerDrift.driftFetchJSON for the shared fetch
// helper and reads window.MIDASExplorerDriftWorkbench for the
// workbench's observation-row deselect callback.

(function () {
  'use strict';

  // ── DOM / template helpers ─────────────────────────────────────────

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
    return map[status] || '';
  }

  // ── Fetch helpers — Drift-1d read endpoints only ───────────────────

  function fetchJSON(url) {
    if (window.MIDASExplorerDrift && typeof window.MIDASExplorerDrift.driftFetchJSON === 'function') {
      return window.MIDASExplorerDrift.driftFetchJSON(url);
    }
    return Promise.resolve({ ok: false, status: 0, json: null });
  }

  function fetchObservation(observationID) {
    return fetchJSON('/v1/drift/observations/' + encodeURIComponent(observationID));
  }
  function fetchObservationAnnotations(observationID) {
    return fetchJSON('/v1/drift/observations/' + encodeURIComponent(observationID) + '/annotations');
  }
  function fetchSeriesPoint(pointID) {
    return fetchJSON('/v1/drift/series-points/' + encodeURIComponent(pointID));
  }

  // ── Module state ───────────────────────────────────────────────────

  var state = {
    selectedObservationID: null,
    selectedObservation: null,
    selectedObservationPoint: null,
    selectedObservationAnnotations: [],
    observationInspectorError: null,
    annotationsLoadError: false,
    pointLoadError: false,
  };

  // ── Renderers — every section is read-only ─────────────────────────

  function renderHeader(host) {
    if (!host) return;
    var o = state.selectedObservation;
    if (!o) {
      host.innerHTML =
        '<div class="drift-observation-inspector-title">Observation</div>'
        + '<div class="drift-observation-inspector-subtitle">Select an observation to inspect its drift evidence.</div>'
        + '<button type="button" class="drift-observation-inspector-close"'
          + ' data-drift-observation-inspector-close'
          + ' aria-label="Close inspector">Close</button>';
      return;
    }
    var detectorCls = statusClassFor(o.detector_status);
    host.innerHTML =
      '<div class="drift-observation-inspector-titles">'
        + '<div class="drift-observation-inspector-title">Observation</div>'
        + '<div class="drift-observation-inspector-subtitle">'
          + '<code class="drift-observation-code-chip">' + escapeHTML(o.id || '—') + '</code>'
          + ' · <span class="drift-observation-drift-type">' + escapeHTML(o.drift_type || '—') + '</span>'
          + ' · ' + escapeHTML((o.target && o.target.kind) || '—')
          + ' / ' + escapeHTML((o.target && o.target.id) || '—')
        + '</div>'
        + '<div class="drift-observation-inspector-status-row">'
          + '<span class="drift-observation-detector-status ' + detectorCls + '">'
            + 'detector: ' + escapeHTML(o.detector_status || '—')
          + '</span>'
          + '<span class="drift-observation-operator-status">'
            + 'operator: ' + escapeHTML(o.operator_status || '—')
          + '</span>'
        + '</div>'
      + '</div>'
      + '<button type="button" class="drift-observation-inspector-close"'
        + ' data-drift-observation-inspector-close'
        + ' aria-label="Close inspector">Close</button>';
  }

  function kvRow(key, value) {
    return '<div class="drift-observation-kv">'
      + '<span class="drift-observation-kv-key">' + escapeHTML(key) + '</span>'
      + '<span class="drift-observation-kv-value">'
        + escapeHTML(String(value === undefined || value === null || value === '' ? '—' : value))
      + '</span>'
    + '</div>';
  }

  function renderWindowAndMagnitude(o) {
    if (!o) return '';
    return ''
      + '<section class="drift-observation-section">'
        + '<h3 class="drift-observation-section-title">Window &amp; magnitude</h3>'
        + kvRow('observed_window_start', o.observed_window_start)
        + kvRow('observed_window_end', o.observed_window_end)
        + kvRow('magnitude', o.magnitude)
        + kvRow('baseline_window_id', o.baseline_window_id)
        + kvRow('detected_at', o.detected_at)
        + kvRow('emitted_at', o.emitted_at)
      + '</section>';
  }

  function renderLinkedPoint(o) {
    if (!o) return '';
    var html = '<section class="drift-observation-section">'
      + '<h3 class="drift-observation-section-title">Linked point</h3>';
    var p = state.selectedObservationPoint;
    if (state.pointLoadError && !p) {
      html += '<div class="drift-observation-empty">Linked series point could not be loaded.</div>';
    } else if (!p) {
      html += '<div class="drift-observation-empty">Linked series point could not be loaded.</div>';
    } else {
      html += kvRow('point id', p.id);
      html += kvRow('series id', p.series_id);
      html += kvRow('window_start', p.window_start);
      html += kvRow('window_end', p.window_end);
      html += kvRow('sample_count', p.sample_count);
      html += kvRow('status', p.status);
      html += kvRow('computation_mode', p.computation_mode);
      html += kvRow('computed_at', p.computed_at);
      html += kvRow('source_window_complete',
        p.source_window_complete === undefined ? '—' : (p.source_window_complete ? 'yes' : 'no'));
    }
    html += '</section>';
    return html;
  }

  function renderBackfill(o) {
    if (!o) return '';
    var html = '<section class="drift-observation-section">'
      + '<h3 class="drift-observation-section-title">Backfill</h3>';
    if (o.backfilled || (o.backfill_run_id && o.backfill_run_id !== '')) {
      // Backfill badge is a separate DOM element with its own class.
      // It is NEVER folded into a status modifier — backfill is
      // orthogonal to detector status.
      html += '<div class="drift-observation-backfill-badge"'
        + ' aria-label="Backfilled observation">Backfilled observation</div>';
      html += kvRow('backfill_run_id', o.backfill_run_id);
    } else {
      html += '<div class="drift-observation-empty">Realtime observation; no backfill provenance.</div>';
    }
    html += '</section>';
    return html;
  }

  function renderEvidenceRefs(o) {
    if (!o) return '';
    var html = '<section class="drift-observation-section">'
      + '<h3 class="drift-observation-section-title">Evidence references</h3>';
    var ids = (o.evidence_envelope_ids || []);
    if (ids.length === 0) {
      html += '<div class="drift-observation-empty">No evidence envelope references recorded.</div>';
    } else {
      html += '<ul class="drift-observation-evidence-list">';
      for (var i = 0; i < ids.length; i++) {
        html += '<li><code class="drift-observation-code-chip">' + escapeHTML(ids[i]) + '</code></li>';
      }
      html += '</ul>';
    }
    html += '</section>';
    return html;
  }

  function renderGovernanceRefs(o) {
    if (!o) return '';
    var html = '<section class="drift-observation-section drift-observation-governance-refs">'
      + '<h3 class="drift-observation-section-title">Governance references</h3>';
    var fmpID = o.related_fail_mode_policy_id || '';
    var expRef = o.related_governance_exp_ref || '';
    if (fmpID === '' && expRef === '') {
      html += '<div class="drift-observation-empty">No related governance references recorded.</div>';
    } else {
      if (fmpID !== '') {
        html += kvRow('related_fail_mode_policy_id', fmpID);
      }
      if (expRef !== '') {
        html += kvRow('related_governance_exp_ref', expRef);
      }
    }
    html += '</section>';
    return html;
  }

  function renderAnnotationsSection() {
    var html = '<section class="drift-observation-section">'
      + '<h3 class="drift-observation-section-title">Annotations</h3>';
    if (state.annotationsLoadError) {
      html += '<div class="drift-observation-empty">Observation annotations could not be loaded.</div>';
    } else {
      var annos = state.selectedObservationAnnotations || [];
      if (annos.length === 0) {
        html += '<div class="drift-observation-empty">No annotations recorded for this observation.</div>';
      } else {
        var sorted = annos.slice().sort(function (a, b) {
          return (Date.parse(b.created_at || '') || 0) - (Date.parse(a.created_at || '') || 0);
        });
        for (var i = 0; i < sorted.length; i++) {
          var a = sorted[i];
          html += '<article class="drift-observation-annotation-card">'
            + '<div class="drift-observation-annotation-meta">'
              + '<span class="drift-observation-annotation-type">' + escapeHTML(a.annotation_type || '—') + '</span>'
              + '<span class="drift-observation-annotation-status">' + escapeHTML(a.status || '—') + '</span>'
              + '<span class="drift-observation-annotation-author">' + escapeHTML(a.author_id || '—') + '</span>'
              + '<span class="drift-observation-annotation-time">' + escapeHTML(a.created_at || '—') + '</span>'
            + '</div>'
            + '<div class="drift-observation-annotation-body">' + escapeHTML(a.body || '') + '</div>'
          + '</article>';
        }
      }
    }
    html += '</section>';
    return html;
  }

  function renderBody(host) {
    if (!host) return;
    var o = state.selectedObservation;
    if (state.observationInspectorError && !o) {
      host.innerHTML = '<div class="drift-observation-empty">Observation details could not be loaded.</div>';
      return;
    }
    if (!o) {
      host.innerHTML = '';
      return;
    }
    host.innerHTML = ''
      + renderWindowAndMagnitude(o)
      + renderLinkedPoint(o)
      + renderBackfill(o)
      + renderEvidenceRefs(o)
      + renderGovernanceRefs(o)
      + renderAnnotationsSection();
  }

  function renderInspector() {
    var root = document.getElementById('drift-observation-inspector');
    if (!root) return;
    var headerHost = root.querySelector('[data-drift-observation-inspector-header]');
    var bodyHost = root.querySelector('[data-drift-observation-inspector-body]');
    renderHeader(headerHost);
    renderBody(bodyHost);
  }

  // ── Public entry points ────────────────────────────────────────────

  function showInspector() {
    var root = document.getElementById('drift-observation-inspector');
    if (root) root.removeAttribute('hidden');
  }
  function hideInspector() {
    var root = document.getElementById('drift-observation-inspector');
    if (root) {
      root.setAttribute('hidden', '');
    }
  }

  function openObservationInspector(observationID, cachedObservation) {
    if (!observationID) return;
    state.selectedObservationID = observationID;
    state.selectedObservation = cachedObservation || null;
    state.selectedObservationPoint = null;
    state.selectedObservationAnnotations = [];
    state.observationInspectorError = null;
    state.annotationsLoadError = false;
    state.pointLoadError = false;
    showInspector();
    renderInspector();

    // Defensive re-fetch — the cached payload may be stale or partial.
    fetchObservation(observationID).then(function (res) {
      if (state.selectedObservationID !== observationID) return;
      if (!res || !res.ok || !res.json) {
        state.observationInspectorError = 'observation_load_failed';
        renderInspector();
        return;
      }
      state.selectedObservation = res.json;
      renderInspector();
      // Linked-point fetch — only after we know the canonical point id.
      if (state.selectedObservation.point_id) {
        fetchSeriesPoint(state.selectedObservation.point_id).then(function (pres) {
          if (state.selectedObservationID !== observationID) return;
          if (!pres || !pres.ok || !pres.json) {
            state.pointLoadError = true;
            renderInspector();
            return;
          }
          state.selectedObservationPoint = pres.json;
          renderInspector();
        });
      }
    });

    // Annotations fetch is independent of the observation re-fetch.
    fetchObservationAnnotations(observationID).then(function (ares) {
      if (state.selectedObservationID !== observationID) return;
      if (!ares || !ares.ok || !ares.json) {
        state.annotationsLoadError = true;
        renderInspector();
        return;
      }
      state.selectedObservationAnnotations = ares.json.drift_annotations || [];
      renderInspector();
    });
  }

  function closeObservationInspector() {
    state.selectedObservationID = null;
    state.selectedObservation = null;
    state.selectedObservationPoint = null;
    state.selectedObservationAnnotations = [];
    state.observationInspectorError = null;
    state.annotationsLoadError = false;
    state.pointLoadError = false;
    hideInspector();
  }

  // ── Wire delegated events on the inspector root ────────────────────
  // Close button reaches into the workbench module to clear the
  // selected observation row visual state.

  function wireInspectorEvents() {
    var root = document.getElementById('drift-observation-inspector');
    if (!root) return;
    root.addEventListener('click', function (e) {
      var target = e.target;
      var closeBtn = target && target.closest
        ? target.closest('[data-drift-observation-inspector-close]')
        : null;
      if (closeBtn) {
        if (window.MIDASExplorerDriftWorkbench
            && typeof window.MIDASExplorerDriftWorkbench._clearSelectedObservation === 'function') {
          window.MIDASExplorerDriftWorkbench._clearSelectedObservation();
        } else {
          closeObservationInspector();
        }
      }
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireInspectorEvents);
  } else {
    wireInspectorEvents();
  }

  window.MIDASExplorerDriftObservationInspector = {
    openObservationInspector: openObservationInspector,
    closeObservationInspector: closeObservationInspector,
    fetchObservation: fetchObservation,
    fetchObservationAnnotations: fetchObservationAnnotations,
    fetchSeriesPoint: fetchSeriesPoint,
  };
})();
