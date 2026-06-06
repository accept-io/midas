// /explorer/assets/js/graph/context/context-evidence-tray.js â€” D32a-impl-9
//
// Context Graph evidence/drift tray. Production owner of the bottom
// evidence-tray UI for the Context lens, formerly inline in
// index.html (~990 lines moved out in D32a-impl-9).
//
// What the tray owns:
//   â€¢ Drift panel:
//     - chart (SVG line chart of a synthetic time-series per
//       selected node + metric + range)
//     - signal-list tiles (signal band, driver, baseline â†’ current)
//     - exposure roll-up for structural node kinds
//   â€¢ Activity panel: runtime envelope feed filtered for the selected
//     node's BS / process / surface context.
//   â€¢ Evidence panel: placeholder for future search/packet integration.
//   â€¢ Header: title, metric selector, range selector.
//   â€¢ Tab switcher: drift / evidence / activity / overview.
//   â€¢ Tray toggle: collapse / expand.
//
// External dependencies (read-only):
//   â€¢ window.MIDASExplorerGraph.state.selectedId    (D32a-impl-3..4)
//   â€¢ window.MIDASExplorerGraph.state.positions     (D32a-impl-3)
//   â€¢ window.MIDASExplorerUtils.escHtml / formatGmapDemoValue /
//     bandClassFor / formatRecordTimestamp / recordsOutcomeClass
//   â€¢ window.MIDASExplorerRecords.mapExplorerEnvelopeToRecordRow
//
// External dependencies (via hooks):
//   â€¢ getGmapData()           â€” inline gmapData (the adapter-shaped
//                               layout the Context view rendered)
//   â€¢ getCurrentGraphView()   â€” inline currentGraphView
//   â€¢ getCurrentGraphRootId() â€” inline currentGraphRootId
//   â€¢ focusGmapOnNode(id)     â€” inline focusGmapOnNode (camera)
//   â€¢ selectNode(id)          â€” inline selectGovernanceMapNode
//                               (inspector orchestration)
//
// Public surface on window.MIDASExplorerGraph.contextEvidenceTray:
//   init(options)
//     Wires the tray DOM (selectors + toggle + tabs + activity
//     refresh on records updates). Accepts options.hooks bundle.
//   notifySelectionChanged()
//     Called by context-graph-inspector.js when the selected node
//     changes. Re-renders the active tab.
//   applyState()
//     Applies expanded / collapsed class + repopulates the active
//     panel. Idempotent.
//   render(node, ctx)
//     Lens-agnostic inspector dispatch entry-point (currently no-op;
//     the tray follows selection changes through
//     notifySelectionChanged rather than per-render dispatch).
//
// Tray state lives module-private (gmapEvidenceTrayExpanded,
// gmapEvidenceTrayActiveTab, gmapEvidenceActivity*). The inline IIFE
// no longer carries any tray state.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // â”€â”€ Module utilities â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
  var _hooks = {};

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _records() { return window.MIDASExplorerRecords || {}; }
  function _state()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.state) || {}; }

  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _formatGmapDemoValue(v, unit) {
    var fn = _utils().formatGmapDemoValue;
    return typeof fn === 'function' ? fn(v, unit) : String(v) + (unit || '');
  }
  function _bandClassFor(band) {
    var fn = _utils().bandClassFor;
    return typeof fn === 'function' ? fn(band) : '';
  }
  function _formatRecordTimestamp(ts) {
    var fn = _utils().formatRecordTimestamp;
    return typeof fn === 'function' ? fn(ts) : String(ts || '');
  }
  function _recordsOutcomeClass(o) {
    var fn = _utils().recordsOutcomeClass;
    return typeof fn === 'function' ? fn(o) : '';
  }
  function _mapExplorerEnvelopeToRecordRow(env) {
    var fn = _records().mapExplorerEnvelopeToRecordRow;
    return typeof fn === 'function' ? fn(env) : null;
  }
  function _getSelectedId()        { return _state().selectedId || null; }
  function _getPositions()         { return _state().positions || {}; }
  function _getGmapData()          { return (typeof _hooks.getGmapData === 'function') ? _hooks.getGmapData() : null; }
  function _getCurrentGraphView()  { return (typeof _hooks.getCurrentGraphView === 'function') ? _hooks.getCurrentGraphView() : 'service'; }
  function _getCurrentGraphRootId(){ return (typeof _hooks.getCurrentGraphRootId === 'function') ? _hooks.getCurrentGraphRootId() : ''; }
  function _focusGmapOnNode(id)    { if (typeof _hooks.focusGmapOnNode === 'function') _hooks.focusGmapOnNode(id); }
  function _selectGovernanceMapNode(id) { if (typeof _hooks.selectNode === 'function') _hooks.selectNode(id); }

  // â”€â”€ Tray module-private state â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

  let gmapEvidenceTrayExpanded = false;
  let gmapEvidenceTrayActiveTab = 'drift';

  // D26c â€” Activity tab runtime feed state.
  let gmapEvidenceActivityItems       = [];
  let gmapEvidenceActivityLoading     = false;
  let gmapEvidenceActivityError       = '';
  let gmapEvidenceActivityLoadedOnce  = false;

  // â”€â”€ Tray functions (transcribed verbatim from inline IIFE) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€


  // hashGmapDemoSeed turns an arbitrary string into a non-negative
  // 32-bit integer. Cheap deterministic hash (djb2 variant) â€” same
  // string always produces the same seed, so the synthetic time-
  // series stays stable across re-renders for a given node + metric.
  function hashGmapDemoSeed(s) {
    let h = 5381;
    const str = String(s || '');
    for (let i = 0; i < str.length; i++) {
      h = ((h << 5) + h + str.charCodeAt(i)) | 0;
    }
    return Math.abs(h);
  }

  // buildDemoDriftSeries produces a deterministic synthetic time-
  // series for the given (nodeId, metric, range) tuple. Returns:
  //   { points: [{t, v}], min, max, label, unit, n }
  // where points has length matching the range (24 / 7 / 30).
  // The base value is metric-appropriate (escalation-rate 5â€“15%,
  // decision-volume 100â€“1000, evidence-completeness 70â€“95%); each
  // point varies by Â±15% of base via a seeded sine wave with a
  // secondary harmonic, so the series looks "plausibly real" rather
  // than flat or pure noise.
  function buildDemoDriftSeries(nodeId, metric, range) {
    const seed = hashGmapDemoSeed((nodeId || 'unknown') + '|' + (metric || 'escalation-rate'));
    let baseLow, baseHigh, label, unit;
    switch (metric) {
      case 'decision-volume':
        baseLow = 100; baseHigh = 1000; label = 'Decision volume'; unit = '';
        break;
      case 'evidence-completeness':
        baseLow = 70; baseHigh = 95; label = 'Evidence completeness'; unit = '%';
        break;
      case 'escalation-rate':
      default:
        baseLow = 5; baseHigh = 15; label = 'Escalation rate'; unit = '%';
        break;
    }
    let n;
    switch (range) {
      case '24h': n = 24; break;
      case '30d': n = 30; break;
      case '7d':
      default: n = 7; break;
    }
    const base = baseLow + (seed % 1000) / 1000 * (baseHigh - baseLow);
    const phase = (seed % 360) * Math.PI / 180;
    const harmonicAmp = 0.06 + (seed % 100) / 1000;
    const points = [];
    let minV = Infinity, maxV = -Infinity;
    for (let i = 0; i < n; i++) {
      const t = i / Math.max(1, n - 1);
      const primary = Math.sin(phase + t * Math.PI * 2) * 0.10;
      const harmonic = Math.sin(phase * 1.7 + t * Math.PI * 4) * harmonicAmp;
      const variation = primary + harmonic;
      const v = Math.max(0, base * (1 + variation));
      points.push({ t: i, v: v });
      if (v < minV) minV = v;
      if (v > maxV) maxV = v;
    }
    if (minV === Infinity) { minV = 0; maxV = 1; }
    if (maxV - minV < 0.5) { maxV = minV + 1; }
    return { points: points, min: minV, max: maxV, label: label, unit: unit, n: n };
  }

  // _bandClassFor maps a band string to its colour class. Shared by
  // exposure and direct-drift tile renderers so the visual treatment
  // is consistent.
  // Phase 2B Step 40 (D25e) â€” node-kind â†’ tray semantics mapping.
  //
  // The MIDAS Governance Drift Design Model (D25d) specifies that
  // only runtime-bearing nodes can directly drift; structural nodes
  // have drift exposure. The tray must follow the rule by displaying
  // kind-specific labels, tiles, charts, and copy.
  //
  // signalClass values:
  //   'direct_drift'   â€” runtime-bearing entity with a measurable
  //                      time-series metric. Example: Decision Surface,
  //                      Coverage. Tray shows baseline â†’ current,
  //                      driver, window, chart.
  //   'usage_drift'    â€” AI System usage / outcome drift across
  //                      bindings. Tray shows usage signal, affected
  //                      bindings, primary surface, driver.
  //   'exposure'       â€” structural entity (BS, Capability, Process,
  //                      Related Service, Authority synthetic). Tray
  //                      shows exposure roll-up: affected children,
  //                      primary contributor, top driver. NO direct
  //                      metric chart.
  //   'preview'        â€” unknown kind. Tray shows a generic preview
  //                      placeholder.
  //
  // The semantics object is consumed by the tile renderer, the chart
  // renderer (which omits the chart for exposure), the metric selector
  // (which is hidden for exposure), and the tray title.
  function getGmapEvidenceSignalSemantics(nodeId) {
    const pos = nodeId ? _getPositions()[nodeId] : null;
    const kind = pos && pos.kind ? pos.kind : '';
    switch (kind) {
      case 'surface':
        return {
          kind: 'surface',
          signalClass: 'direct_drift',
          title: 'Decision surface drift',
          subtitle: 'Direct drift on a runtime-bearing decision surface. Synthetic illustrative signal.',
          summaryTileLabels: ['Signal', 'Driver', 'Baseline â†’ Current', 'Window / Volume'],
          chartMode: 'metric_timeseries',
          metricOptions: [
            { value: 'escalation-rate',       label: 'Escalation rate' },
            { value: 'evidence-completeness', label: 'Evidence completeness' },
            { value: 'decision-volume',       label: 'Decision volume' },
          ],
          emptyText: 'No drift signals for this surface in the selected window.',
          isDirectDrift: true,
          isExposure: false,
        };
      case 'ai':
        return {
          kind: 'ai',
          signalClass: 'usage_drift',
          title: 'AI usage / outcome drift',
          subtitle: 'AI usage / outcome drift across bindings. Synthetic illustrative signal.',
          summaryTileLabels: ['Usage signal', 'Affected bindings', 'Primary surface', 'Driver'],
          chartMode: 'usage_timeseries',
          metricOptions: [
            { value: 'invocation-share',         label: 'Invocation share' },
            { value: 'binding-outcome-divergence', label: 'Binding outcome divergence' },
            { value: 'version-mix',              label: 'Version mix' },
          ],
          emptyText: 'No usage / outcome drift across bindings in the selected window.',
          isDirectDrift: true,
          isExposure: false,
        };
      case 'coverage':
        return {
          kind: 'coverage',
          signalClass: 'direct_drift',
          title: 'Coverage drift',
          subtitle: 'Coverage-gap event trend. Synthetic illustrative signal.',
          summaryTileLabels: ['Coverage signal', 'Gap-event rate', 'Affected surfaces', 'Window'],
          chartMode: 'metric_timeseries',
          metricOptions: [
            { value: 'coverage-gap-rate',  label: 'Coverage gap rate' },
            { value: 'conditions-detected', label: 'Conditions detected' },
          ],
          emptyText: 'No coverage signals in the selected window.',
          isDirectDrift: true,
          isExposure: false,
        };
      case 'authority':
        return {
          kind: 'authority',
          signalClass: 'exposure',
          title: 'Authority drift exposure',
          subtitle: 'Authority drift exposure roll-up across associated profiles and grants. Synthetic illustrative signal.',
          summaryTileLabels: ['Authority exposure', 'Affected profiles', 'Affected grants', 'Primary driver'],
          chartMode: 'exposure_explanation',
          metricOptions: [],
          emptyText: 'No authority drift signals in the selected window.',
          isDirectDrift: false,
          isExposure: true,
        };
      case 'business':
        return {
          kind: 'business',
          signalClass: 'exposure',
          title: 'Service drift exposure',
          subtitle: 'Service drift exposure: synthetic roll-up across associated decision surfaces.',
          summaryTileLabels: ['Exposure', 'Affected surfaces', 'Primary driver', 'Highest contributor'],
          chartMode: 'exposure_explanation',
          metricOptions: [],
          emptyText: 'No drift signals across this serviceâ€™s surfaces in the selected window.',
          isDirectDrift: false,
          isExposure: true,
        };
      case 'related':
        return {
          kind: 'related',
          signalClass: 'exposure',
          title: 'Related-service drift exposure',
          subtitle: 'Related-service drift exposure roll-up across associated surfaces.',
          summaryTileLabels: ['Exposure', 'Affected surfaces', 'Primary driver', 'Highest contributor'],
          chartMode: 'exposure_explanation',
          metricOptions: [],
          emptyText: 'No drift signals across this related serviceâ€™s surfaces in the selected window.',
          isDirectDrift: false,
          isExposure: true,
        };
      case 'cap':
        return {
          kind: 'cap',
          signalClass: 'exposure',
          title: 'Capability drift exposure',
          subtitle: 'Capability exposure is derived from associated decision surfaces, not from the capability itself.',
          summaryTileLabels: ['Exposure', 'Affected surfaces', 'Highest child signal', 'Primary contributor'],
          chartMode: 'exposure_explanation',
          metricOptions: [],
          emptyText: 'No drift signals across this capabilityâ€™s enabling surfaces in the selected window.',
          isDirectDrift: false,
          isExposure: true,
        };
      case 'proc':
        return {
          kind: 'proc',
          signalClass: 'exposure',
          title: 'Process drift signals',
          subtitle: 'Process drift signals derived from decision surfaces under this process.',
          summaryTileLabels: ['Signals', 'Affected surfaces', 'Top driver', 'Primary surface'],
          chartMode: 'exposure_explanation',
          metricOptions: [],
          emptyText: 'No drift signals on surfaces under this process in the selected window.',
          isDirectDrift: false,
          isExposure: true,
        };
      default:
        return {
          kind: kind || 'unknown',
          signalClass: 'preview',
          title: 'Runtime signal preview',
          subtitle: 'Runtime signal preview. Synthetic illustrative content.',
          summaryTileLabels: ['Status'],
          chartMode: 'none',
          metricOptions: [],
          emptyText: 'Select a runtime-bearing or structural node to inspect signals.',
          isDirectDrift: false,
          isExposure: false,
        };
    }
  }

  // Phase 2B Step 40 (D25e) â€” semantic synthetic generator wrapper.
  //
  // buildDemoGovernanceSignal produces a deterministic synthetic
  // signal object whose SHAPE depends on the node kind's signal
  // class. The shape mirrors the design model (D25d Â§15) so the
  // future analytics endpoint can swap in real values without the
  // tray having to change its rendering paths.
  //
  // Direct drift / usage drift signals carry: baseline, current,
  // delta, driver, window, volume.
  // Exposure signals carry: affected_count, total_count,
  // primary_driver, primary_contributor, exposure_band.
  //
  // Determinism â€” every input producing this signal seeds the same
  // hashGmapDemoSeed, so reload / re-render produces identical output.
  // No Math.random anywhere in this function or its callees.
  function buildDemoGovernanceSignal(nodeId, semantics, metric, range) {
    const seed = hashGmapDemoSeed(
      (nodeId || 'unknown') + '|' + (semantics.kind || 'unknown') + '|' + (metric || semantics.signalClass)
    );
    if (semantics.signalClass === 'exposure') {
      // Synthetic exposure roll-up. Counts are bounded by the kind:
      // a typical BS has 2-4 child surfaces; a Capability spans
      // 1-3 surfaces; a Process owns 1-2; Authority synthetic spans
      // a few profiles + grants.
      let totalLow, totalHigh;
      switch (semantics.kind) {
        case 'business':  totalLow = 2; totalHigh = 4; break;
        case 'cap':       totalLow = 1; totalHigh = 3; break;
        case 'proc':      totalLow = 1; totalHigh = 2; break;
        case 'authority': totalLow = 1; totalHigh = 3; break;
        default:          totalLow = 1; totalHigh = 3;
      }
      const total    = totalLow + (seed % (totalHigh - totalLow + 1));
      const affected = Math.min(total, ((seed >> 3) % (total + 1)));
      const drivers  = ['outcome drift', 'evidence drift', 'authority drift', 'coverage drift'];
      const primaryDriver = drivers[(seed >> 5) % drivers.length];
      const contributors  = ['primary contributor A', 'primary contributor B', 'primary contributor C'];
      const primaryContributor = contributors[(seed >> 7) % contributors.length];
      let band;
      if      (affected === 0)        band = 'No exposure';
      else if (affected < total / 2)  band = 'Watch';
      else if (affected < total)      band = 'Drifting';
      else                            band = 'Critical';
      return {
        signalClass:         'exposure',
        affected_count:      affected,
        total_count:         total,
        primary_driver:      primaryDriver,
        primary_contributor: primaryContributor,
        exposure_band:       band,
        demo:                true,
      };
    }
    // Direct drift / usage drift / preview: derive baseline + current
    // from the existing buildDemoDriftSeries (which gives a stable
    // time-series). Latest point becomes the current value; the median
    // of the window becomes the synthetic baseline.
    const seriesMetric = metric || (semantics.metricOptions[0] && semantics.metricOptions[0].value) || 'escalation-rate';
    const series = buildDemoDriftSeries(nodeId, seriesMetric, range || '7d');
    const current  = series.points[series.points.length - 1].v;
    // Median of the window as the baseline anchor â€” stable, simple,
    // honest about its synthetic origin.
    const sorted   = series.points.map(function (p) { return p.v; }).slice().sort(function (a, b) { return a - b; });
    const baseline = sorted[Math.floor(sorted.length / 2)];
    const delta    = current - baseline;
    const drivers = ['consequence', 'confidence', 'policy', 'context'];
    const driver  = drivers[(seed >> 4) % drivers.length];
    let windowLabel;
    switch (range || '7d') {
      case '24h': windowLabel = '24h'; break;
      case '30d': windowLabel = '30d'; break;
      default:    windowLabel = '7d';
    }
    // Synthetic volume â€” proportional to seed so larger graphs hash
    // to higher volumes; bounded so it stays readable.
    const volume = 100 + (seed % 4000);
    return {
      signalClass: semantics.signalClass,
      metric:      seriesMetric,
      label:       series.label,
      unit:        series.unit,
      baseline:    baseline,
      current:     current,
      delta:       delta,
      driver:      driver,
      window:      windowLabel,
      volume:      volume,
      series:      series,
      demo:        true,
    };
  }

  // gmapNodeKindLabel returns a human-readable kind string for the
  // selected node, drawn from _getPositions()[nodeId].kind. Falls back
  // to "Node" when the kind is missing or unrecognised.
  function gmapNodeKindLabel(nodeId) {
    const pos = nodeId ? _getPositions()[nodeId] : null;
    if (!pos) return '';
    switch (pos.kind) {
      case 'business':  return 'Business Service';
      case 'related':   return 'Related Service';
      case 'cap':       return 'Capability';
      case 'proc':      return 'Process';
      case 'surface':   return 'Decision Surface';
      case 'ai':        return 'AI System';
      case 'authority': return 'Authority';
      case 'coverage':  return 'Coverage';
      case 'more':      return 'More';
      default:          return 'Node';
    }
  }

  // updateGmapEvidenceTrayHeader writes the selected-node text into
  // the header. When no node is selected, the placeholder reverts.
  // The kind label sits beside the name for context.
  function updateGmapEvidenceTrayHeader() {
    const nodeEl = document.getElementById('gmap-evidence-tray-node');
    if (!nodeEl) return;
    const nodeId = _getSelectedId();
    if (!nodeId || !_getPositions()[nodeId]) {
      nodeEl.textContent = 'Select a graph node to inspect runtime evidence.';
      return;
    }
    // Read the rendered DOM card's data-node-name (the canonical
    // display name set by addNode); fall back to id when missing.
    const canvas = document.getElementById('gmap-canvas');
    let displayName = nodeId;
    if (canvas) {
      const nodes = canvas.querySelectorAll('.gmap-node');
      for (let i = 0; i < nodes.length; i++) {
        if (nodes[i].dataset.nodeId === nodeId) {
          displayName = nodes[i].dataset.nodeName || nodeId;
          break;
        }
      }
    }
    const kind = gmapNodeKindLabel(nodeId);
    nodeEl.innerHTML = '<span class="gmap-evidence-tray-node-kind">' + _escHtml(kind) + '</span> ' +
      '<strong>' + _escHtml(displayName) + '</strong>';
  }

  // Phase 2B Step 40 (D25e) â€” render the drift panel's content shape
  // for the currently-selected node. For runtime-bearing nodes
  // (direct_drift, usage_drift) the panel includes metric + range
  // selectors, summary tiles, and a time-series chart. For exposure
  // nodes the panel hides the metric selector + chart and shows the
  // exposure tiles + an explanatory copy block. For unknown kinds it
  // shows the preview placeholder. The caller (the tab switcher) is
  // responsible for ensuring this only runs while the Drift tab is
  // active.
  function renderGmapEvidenceTrayDriftPanel() {
    const panel = document.getElementById('gmap-evidence-tray-panel');
    if (!panel) return;
    // D32e-impl-1 â€” Delegate Drift tab rendering to the Drift
    // Analytics panel module when present. The module owns the
    // chart + series list + adapter contract; the tray's only job
    // for the Drift tab is to ensure the analytics layout is
    // present in the panel mount and to invoke render().
    if (window.MIDASExplorerDriftAnalytics &&
        typeof window.MIDASExplorerDriftAnalytics.render === 'function') {
      // Restore the analytics layout if a previous tab switch
      // overwrote the panel innerHTML.
      if (!panel.querySelector('[data-drift-compact-summary]')) {
        panel.innerHTML =
          '<div class="drift-analytics-panel">' +
            '<div class="drift-compact-summary" data-drift-compact-summary></div>' +
          '</div>';
      }
      try { window.MIDASExplorerDriftAnalytics.render(); } catch (_) { /* swallow */ }
    }
    return;
  }

  // notifyGmapEvidenceTraySelectionChanged is called from
  // _selectGovernanceMapNode whenever the primary selection changes.
  // It refreshes the tray header always, and the panel content only
  // when the tray is expanded AND a kind-aware tab is active (the panel
  // is hidden when collapsed, so re-rendering it then is wasted DOM
  // work). D25e â€” the Drift panel rebuilds its shape because different
  // node kinds produce different layouts. D26c â€” the Activity panel
  // also re-renders so its local selection filter (surface/business/
  // process) reflects the new node, but it does NOT re-fetch since the
  // session-level item set is unchanged.
  function notifyGmapEvidenceTraySelectionChanged() {
    updateGmapEvidenceTrayHeader();
    if (!gmapEvidenceTrayExpanded) return;
    if (gmapEvidenceTrayActiveTab === 'drift') {
      renderGmapEvidenceTrayDriftPanel();
    } else if (gmapEvidenceTrayActiveTab === 'activity') {
      renderGmapEvidenceTrayActivityPanel();
    }
  }

  // â”€â”€ D26c: Activity tab â€” Explorer runtime envelope feed â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
  // The Activity tab consumes the same isolated runtime feed introduced
  // by D26a (GET /explorer/envelopes) and reuses the D26b row mapper
  // (_mapExplorerEnvelopeToRecordRow) so Records and Activity present a
  // consistent view of the same data. Unlike the Drift tab, Activity is
  // real runtime data â€” provenance copy makes that distinction explicit.
  //
  // Selection context: the backend has no node-scoped envelope filter
  // yet, so the renderer applies a local match against the raw D26a
  // fields surface_id / business_service_id / process_id when the
  // selected node's kind allows it. For other kinds (AI System,
  // Capability, Authority, Coverage, More) the panel shows the
  // unfiltered session list with an honest copy line.

  async function loadGmapEvidenceActivity() {
    gmapEvidenceActivityLoading = true;
    gmapEvidenceActivityError   = '';
    if (gmapEvidenceTrayActiveTab === 'activity' && gmapEvidenceTrayExpanded) {
      renderGmapEvidenceTrayActivityPanel();
    }
    try {
      const headers = { 'Accept': 'application/json' };
      const tok = (typeof getToken === 'function') ? getToken() : null;
      if (tok) headers['Authorization'] = 'Bearer ' + tok;
      const res = await fetch('/explorer/envelopes?limit=50', { headers, credentials: 'same-origin' });
      if (!res.ok) {
        gmapEvidenceActivityError = 'Could not load runtime activity';
        gmapEvidenceActivityItems = [];
        return;
      }
      const payload = await res.json();
      const items = (payload && Array.isArray(payload.items)) ? payload.items : [];
      // Keep raw D26a items so the filter helper can read snake_case
      // fields directly. Mapping happens lazily in the renderer.
      gmapEvidenceActivityItems = items.filter(it => it && typeof it === 'object' && it.id);
    } catch (e) {
      gmapEvidenceActivityError = 'Could not load runtime activity';
      gmapEvidenceActivityItems = [];
    } finally {
      gmapEvidenceActivityLoading    = false;
      gmapEvidenceActivityLoadedOnce = true;
      if (gmapEvidenceTrayActiveTab === 'activity' && gmapEvidenceTrayExpanded) {
        renderGmapEvidenceTrayActivityPanel();
      }
    }
  }

  // filterGmapEvidenceActivityForSelection narrows the session-level
  // item list to entries that match the currently-selected graph node
  // when the node's kind has a stable mapping to an envelope summary
  // field. surface â†’ surface_id, business/related â†’ business_service_id,
  // proc â†’ process_id. AI System / Capability / Authority / Coverage /
  // More return the unfiltered list with kind='unsupported' so the
  // renderer can show an honest copy line.
  //
  // Returns: { rows, filtered, kind, emptyForSelection }
  //   - rows                â€” the items the renderer should display
  //   - filtered            â€” true if a local match narrowed the list
  //   - kind                â€” 'session' | 'surface' | 'business' |
  //                            'process' | 'unsupported'
  //   - emptyForSelection   â€” true when the kind supports filtering but
  //                            no items matched (renderer falls back to
  //                            the full session list with a "no match"
  //                            note)
  function filterGmapEvidenceActivityForSelection(items, nodeId) {
    const safe = Array.isArray(items) ? items : [];
    if (!nodeId || !_getPositions()[nodeId]) {
      return { rows: safe, filtered: false, kind: 'session', emptyForSelection: false };
    }
    const pos = _getPositions()[nodeId];
    const colon = nodeId.indexOf(':');
    const rawId = colon >= 0 ? nodeId.slice(colon + 1) : nodeId;
    if (pos.kind === 'surface') {
      const sub = safe.filter(it => it && it.surface_id === rawId);
      return {
        rows: sub.length ? sub : safe,
        filtered: sub.length > 0,
        kind: 'surface',
        emptyForSelection: sub.length === 0,
      };
    }
    if (pos.kind === 'business' || pos.kind === 'related') {
      const sub = safe.filter(it => it && it.business_service_id === rawId);
      return {
        rows: sub.length ? sub : safe,
        filtered: sub.length > 0,
        kind: 'business',
        emptyForSelection: sub.length === 0,
      };
    }
    if (pos.kind === 'proc') {
      const sub = safe.filter(it => it && it.process_id === rawId);
      return {
        rows: sub.length ? sub : safe,
        filtered: sub.length > 0,
        kind: 'process',
        emptyForSelection: sub.length === 0,
      };
    }
    return { rows: safe, filtered: false, kind: 'unsupported', emptyForSelection: false };
  }

  function renderGmapEvidenceTrayActivityPanel() {
    const panel = document.getElementById('gmap-evidence-tray-panel');
    if (!panel) return;

    // Loading / error / empty branches first â€” they short-circuit the
    // selection-filter logic below.
    if (gmapEvidenceActivityLoading && !gmapEvidenceActivityLoadedOnce) {
      panel.innerHTML =
        '<div class="gmap-evidence-tray-empty">Loading runtime activityâ€¦</div>';
      return;
    }
    if (gmapEvidenceActivityError) {
      panel.innerHTML =
        '<div class="gmap-evidence-tray-empty">' + _escHtml(gmapEvidenceActivityError) + '</div>';
      return;
    }
    if (gmapEvidenceActivityLoadedOnce && gmapEvidenceActivityItems.length === 0) {
      panel.innerHTML =
        '<div class="gmap-evidence-tray-subtitle">' +
          'Activity uses real Explorer runtime envelopes. Drift signals remain illustrative until analytics is wired.' +
        '</div>' +
        '<div class="gmap-evidence-tray-empty">' +
          'No runtime activity yet.<br>' +
          'Run an Explorer evaluation to create an envelope.' +
        '</div>';
      return;
    }

    const filter = filterGmapEvidenceActivityForSelection(gmapEvidenceActivityItems, _getSelectedId());
    let provenance;
    if (filter.kind === 'surface') {
      provenance = filter.filtered
        ? 'Locally filtered from recent Explorer runtime envelopes by surface_id match.'
        : 'No matching runtime activity for this selected node in the recent Explorer envelope list. Showing the latest session activity instead.';
    } else if (filter.kind === 'business') {
      provenance = filter.filtered
        ? 'Locally filtered from recent Explorer runtime envelopes by business_service_id match.'
        : 'No matching runtime activity for this selected node in the recent Explorer envelope list. Showing the latest session activity instead.';
    } else if (filter.kind === 'process') {
      provenance = filter.filtered
        ? 'Locally filtered from recent Explorer runtime envelopes by process_id match.'
        : 'No matching runtime activity for this selected node in the recent Explorer envelope list. Showing the latest session activity instead.';
    } else if (filter.kind === 'unsupported') {
      provenance = 'Showing recent Explorer runtime envelopes for this session. Node-scoped filtering for this kind requires a future analytics endpoint.';
    } else {
      provenance = 'Showing recent Explorer runtime envelopes for this session.';
    }

    const subtitle =
      '<div class="gmap-evidence-tray-subtitle">' +
        'Activity uses real Explorer runtime envelopes. Drift signals remain illustrative until analytics is wired.' +
      '</div>';
    const note =
      '<div class="gmap-evidence-tray-disclaimer">' + _escHtml(provenance) + '</div>';

    const list = filter.rows.map(it => {
      const r = _mapExplorerEnvelopeToRecordRow(it) || { id: String(it.id || '') };
      const outcomeCls = r.outcomeClass ? ' ' + r.outcomeClass : '';
      const outcomeText = r.outcome && r.outcome !== 'â€”' ? r.outcome : (r.state || 'â€”');
      const authority = 'profile ' + _escHtml(r.profileId || 'â€”') +
        ' / grant ' + _escHtml(r.grantId || 'â€”') +
        ' / agent ' + _escHtml(r.agent || 'â€”');
      return (
        '<div class="gmap-evidence-tray-activity-row" data-envelope-id="' + _escHtml(r.id) + '">' +
          '<div class="gmap-evidence-tray-activity-row-head">' +
            '<span class="gmap-evidence-tray-activity-time">' + _escHtml(r.time || 'â€”') + '</span>' +
            '<span class="records-outcome-badge' + outcomeCls + '">' + _escHtml(outcomeText) + '</span>' +
            '<span class="gmap-evidence-tray-activity-state">' + _escHtml(r.state || 'â€”') + '</span>' +
          '</div>' +
          '<div class="gmap-evidence-tray-activity-row-body">' +
            '<span><strong>Reason</strong> ' + _escHtml(r.reason || 'â€”') + '</span>' +
            '<span><strong>Surface</strong> ' + _escHtml(r.surface || 'â€”') + '</span>' +
            '<span><strong>Business service</strong> ' + _escHtml(r.bs || 'â€”') + '</span>' +
            '<span><strong>Process</strong> ' + _escHtml(r.processId || 'â€”') + '</span>' +
            '<span><strong>Request</strong> ' + _escHtml(r.requestSource || 'â€”') + '</span>' +
            '<span><strong>Authority</strong> ' + authority + '</span>' +
            '<span class="gmap-evidence-tray-activity-id">id ' + _escHtml(r.id) + '</span>' +
          '</div>' +
        '</div>'
      );
    }).join('');

    panel.innerHTML = subtitle + note +
      '<div class="gmap-evidence-tray-activity-list">' + list + '</div>';
  }

  // applyGmapEvidenceTrayState toggles the expanded class, syncs
  // ARIA, and (on expand) populates the panel + triggers a re-fit
  // so the graph repositions to the smaller canvas-scroll height.
  function applyGmapEvidenceTrayState() {
    const tray = document.getElementById('gmap-evidence-tray');
    const toggle = document.getElementById('gmap-evidence-tray-toggle');
    if (!tray || !toggle) return;
    tray.classList.toggle('is-expanded', gmapEvidenceTrayExpanded);
    toggle.setAttribute('aria-expanded', String(gmapEvidenceTrayExpanded));
    const lbl = gmapEvidenceTrayExpanded ? 'Collapse letterbox' : 'Expand letterbox';
    toggle.setAttribute('aria-label', lbl);
    toggle.setAttribute('title', lbl);
    if (gmapEvidenceTrayExpanded) {
      // Phase 2B Step 40 (D25e) â€” render the kind-aware drift panel
      // (subtitle + disclaimer + tiles + chart-or-explanation). Only
      // the Drift tab gets this treatment; other tabs keep their
      // "Coming soon" placeholders managed by the tab switcher.
      if (gmapEvidenceTrayActiveTab === 'drift') {
        renderGmapEvidenceTrayDriftPanel();
      } else if (gmapEvidenceTrayActiveTab === 'activity') {
        // D26c â€” Activity is real Explorer runtime data. Trigger a load
        // on every expand so newly-evaluated envelopes appear without
        // a full reload, and render the panel immediately so the
        // loading state is visible while the fetch resolves.
        renderGmapEvidenceTrayActivityPanel();
        loadGmapEvidenceActivity();
      }
    }
    // Re-fit so the graph repositions to the new canvas-scroll height.
    // The CSS height transition takes 0.18s; defer the fit to the
    // next animation frame so the layout has settled. fitGmapToBounds
    // reads scrollEl.clientHeight at fire time, so the post-transition
    // height is what it sees. Skip when there's no data yet.
    if (typeof window.requestAnimationFrame === 'function' && typeof fitGmapToBounds === 'function') {
      window.setTimeout(function () {
        if (typeof fitGmapToBounds === 'function') fitGmapToBounds();
      }, 200);
    }
  }

  // Wire the tray's expand/collapse toggle, tab switching, and
  // metric/range selectors. Idempotent IIFE pattern.
  (function wireGmapEvidenceTray() {
    const toggle = document.getElementById('gmap-evidence-tray-toggle');
    if (toggle) {
      toggle.addEventListener('click', function () {
        gmapEvidenceTrayExpanded = !gmapEvidenceTrayExpanded;
        applyGmapEvidenceTrayState();
      });
    }
    const tabs = document.querySelectorAll('.gmap-evidence-tray-tab');
    const panel = document.getElementById('gmap-evidence-tray-panel');
    if (tabs && panel) {
      tabs.forEach(function (tab) {
        tab.addEventListener('click', function () {
          const which = tab.dataset.tab || 'drift';
          gmapEvidenceTrayActiveTab = which;
          tabs.forEach(function (t) {
            const active = t === tab;
            t.classList.toggle('is-active', active);
            t.setAttribute('aria-selected', String(active));
            if (active) {
              t.setAttribute('aria-current', 'page');
            } else {
              t.removeAttribute('aria-current');
            }
          });
          // D25e â€” the Drift tab's content shape depends on the
          // selected node's kind (direct drift / usage drift / exposure
          // / preview). renderGmapEvidenceTrayDriftPanel dispatches on
          // getGmapEvidenceSignalSemantics(nodeId).
          // D26c â€” the Activity tab consumes real Explorer runtime
          // envelopes from the D26a feed.
          if (which === 'drift') {
            renderGmapEvidenceTrayDriftPanel();
          } else if (which === 'activity') {
            renderGmapEvidenceTrayActivityPanel();
            loadGmapEvidenceActivity();
          } else {
            const labelMap = { overview: 'Overview', evidence: 'Evidence' };
            panel.innerHTML = '<div class="gmap-evidence-tray-coming-soon">' +
              _escHtml(labelMap[which] || which) + ' â€” coming soon. Drift signals ship first; ' +
              'overview / evidence panels arrive with the runtime analytics endpoint.' +
              '</div>';
          }
        });
      });
    }
  })();

  // â”€â”€ Public surface â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

  function init(options) {
    options = options || {};
    if (options.hooks && typeof options.hooks === 'object') {
      _hooks = options.hooks;
    }
    // Apply initial tray state + wire DOM event handlers. Idempotent.
    applyGmapEvidenceTrayState();
  }

  function notifySelectionChanged() {
    notifyGmapEvidenceTraySelectionChanged();
  }

  function applyState() {
    applyGmapEvidenceTrayState();
  }

  function openEvidenceTray() {
    gmapEvidenceTrayExpanded = true;
    gmapEvidenceTrayActiveTab = 'evidence';
    const tabs = document.querySelectorAll('.gmap-evidence-tray-tab');
    tabs.forEach(function (tab) {
      const active = tab.dataset && tab.dataset.tab === 'evidence';
      tab.classList.toggle('is-active', active);
      tab.setAttribute('aria-selected', String(active));
      if (active) {
        tab.setAttribute('aria-current', 'page');
      } else {
        tab.removeAttribute('aria-current');
      }
    });
    applyGmapEvidenceTrayState();
    const panel = document.getElementById('gmap-evidence-tray-panel');
    if (panel) {
      panel.innerHTML = '<div class="gmap-evidence-tray-coming-soon">' +
        'Evidence - coming soon. Drift signals ship first; overview / evidence panels arrive with the runtime analytics endpoint.' +
        '</div>';
    }
    return true;
  }

  // Lens-inspector dispatch entry-point: the tray follows selection
  // through notifySelectionChanged, so render(node, ctx) is a no-op.
  function render(/* node, ctx */) {}

  window.MIDASExplorerGraph.contextEvidenceTray = {
    init:                    init,
    notifySelectionChanged:  notifySelectionChanged,
    applyState:              applyState,
    openEvidenceTray:        openEvidenceTray,
    render:                  render,
    // Exposed for direct tests / advanced consumers.
    _hashSeed:               hashGmapDemoSeed,
    _buildDemoDriftSeries:   buildDemoDriftSeries,
    _signalSemantics:        getGmapEvidenceSignalSemantics,
    _filterActivityForSelection: filterGmapEvidenceActivityForSelection,
  };
})();
