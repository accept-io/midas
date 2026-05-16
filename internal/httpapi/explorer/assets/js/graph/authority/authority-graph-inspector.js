// /explorer/assets/js/graph/authority/authority-graph-inspector.js — D32b-impl-1
//
// Authority Graph lens inspector. Sibling to context-graph-inspector.js;
// production owner of Authority-specific inspector content.
//
// Provides basic card-level formatting for every Authority node kind
// using typed-data already present in the graph response. No extra
// entity-detail APIs are called in this tranche — the inspector
// shows what the graph already gives us.
//
// D33a-spike-2g-impl-5f — Inspector ownership cleanup. The Authority
// Inspector tab owns ONLY the selected object's identity + direct
// attributes (Kind / ID / Label / Connected edges + per-kind
// specific fields + collapsed Technical details). Per-node
// diagnostics belong to the Diagnostics tab (rendered by
// `authority-diagnostics-panel.js`); per-surface posture belongs to
// the Posture & Help tab (rendered by
// `authority-surface-posture-panel.js`). The Inspector no longer
// composes a governance overlay; the `_buildOverlayHTML` /
// `_diagnosticsForNode` / `_postureForSurface` / `_axisAttr`
// helpers that produced it were retired here.
//
// External dependencies:
//   window.MIDASExplorerGraph.inspector.setName / setFields / setSummary /
//     setGovernance / setActions / setInlineActions
//   window.MIDASExplorerGraph.selection.setSelected
//   window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel
//
// Public surface (window.MIDASExplorerGraph.authorityInspector):
//   selectNode(nodeId)
//     Drive the inspector rail from an Authority Graph node id.
//     Reads node-DOM dataset (set by the view's addNode) and pushes
//     fields through the lens-agnostic inspector frame.
//   renderNode(node, mount)
//     Lens-dispatch entry-point used by graph-inspector.register.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils()      { return window.MIDASExplorerUtils || {}; }
  function _inspector()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.inspector) || {}; }
  function _selection()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.selection) || {}; }
  function _adapter()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityAdapter) || null; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }

  // ── Per-kind formatters ───────────────────────────────────────────────
  //
  // Each formatter takes the typed-data object from the projection's
  // Node (e.g. node.authority_profile) plus the projection node ref
  // (id / label) and returns an array of [key, value] pairs the
  // lens-agnostic inspector frame renders as rows.
  //
  // The field lists below match D32b-impl-1's required-fields contract:
  // anything in the contract appears here, anything else is dropped.

  // D33a-spike-2g-impl-5e — Per-kind formatters now return two
  // sublists: `specific` (human-useful selected-node attributes
  // for the primary visible pane) and `technical` (raw / debug /
  // numeric fields that move into a collapsed "Technical details"
  // section). The primary block of fields (Kind / ID / Label /
  // Connected edges) is composed separately by `_primaryRows()`
  // below and rendered BEFORE the per-kind specific list, so each
  // formatter only needs to declare the node-specific extras.
  //
  // `name` and `id` are intentionally omitted from `specific` —
  // they're carried by the selected-node title and the primary ID
  // row. Per-kind values that duplicate the title (e.g.
  // `n.id` / `d.name`) move to `technical` only if they carry
  // additional information.

  function _formatBusinessService(d, n) {
    void n;
    return {
      specific: [
        ['status',        d.status || ''],
        ['owner',         d.owner || ''],
        ['service_type',  d.service_type || ''],
        ['external_ref',  d.external_ref ? _obj(d.external_ref) : ''],
      ],
      technical: [
        ['fail_mode_policy_id', d.fail_mode_policy_id || ''],
      ],
    };
  }

  function _formatDecisionSurface(d, n) {
    void n;
    return {
      specific: [
        ['status',                   d.status || ''],
        ['process_id',               d.process_id || ''],
        ['effective_policy_source',  d.effective_policy_source || ''],
        ['effective_policy_id',      d.effective_policy_id || ''],
        ['inherits_bs_policy',       _bool(d.inherits_bs_policy)],
      ],
      technical: [
        ['version',             d.version != null ? String(d.version) : ''],
        ['business_service_id', d.business_service_id || ''],
      ],
    };
  }

  function _formatAuthorityProfile(d, n) {
    void n;
    return {
      specific: [
        ['status',                d.status || ''],
        ['surface_id',            d.surface_id || ''],
        ['escalation_target_id',  d.escalation_target_id || ''],
        ['fail_mode',             d.fail_mode || ''],
      ],
      technical: [
        ['version',                d.version != null ? String(d.version) : ''],
        ['validity_status',        d.validity_status || ''],
        ['confidence_threshold',   d.confidence_threshold != null ? String(d.confidence_threshold) : ''],
        ['consequence_threshold',  d.consequence_threshold != null ? String(d.consequence_threshold) : ''],
        ['escalation_mode',        d.escalation_mode || ''],
      ],
    };
  }

  function _formatAuthorityGrant(d, n) {
    void n;
    return {
      specific: [
        ['status',        d.status || ''],
        ['agent_id',      d.agent_id || ''],
        ['capabilities',  _list(d.capabilities)],
      ],
      technical: [
        ['profile_id',      d.profile_id || ''],
        ['validity_status', d.validity_status || ''],
        ['constraints',     _obj(d.constraints)],
      ],
    };
  }

  function _formatAgent(d, n) {
    void n;
    return {
      specific: [
        ['operational_state', d.operational_state || ''],
        ['type',              d.type || ''],
        ['owner',             d.owner || ''],
      ],
      technical: [
        ['model_version', d.model_version || ''],
      ],
    };
  }

  function _formatFailModePolicy(d, n) {
    void n;
    return {
      specific: [
        ['status',          d.status || ''],
        ['effective_date',  d.effective_date || ''],
        ['business_owner',  d.business_owner || ''],
        ['technical_owner', d.technical_owner || ''],
      ],
      // The raw rule-count map (JSON-shaped) is moved into the
      // technical list below so the primary visible pane is not
      // dominated by it. View Record remains the route to the
      // full record.
      technical: [
        ['version',             d.version != null ? String(d.version) : ''],
        ['effective_until',     d.effective_until || ''],
        ['origin',              d.origin || ''],
        ['managed',             _bool(d.managed)],
        ['rule_count_by_class', _obj(d.rule_count_by_class)],
      ],
    };
  }

  function _formatEscalationTarget(d, n) {
    void n;
    return {
      specific: [
        ['kind',    d.kind || ''],
        ['handle',  d.handle || ''],
        ['status',  d.status || ''],
      ],
      technical: [
        ['version',         d.version != null ? String(d.version) : ''],
        ['effective_date',  d.effective_date || ''],
        ['effective_until', d.effective_until || ''],
        ['business_owner',  d.business_owner || ''],
        ['technical_owner', d.technical_owner || ''],
      ],
    };
  }

  var FORMATTERS = {
    business_service:  _formatBusinessService,
    decision_surface:  _formatDecisionSurface,
    authority_profile: _formatAuthorityProfile,
    authority_grant:   _formatAuthorityGrant,
    agent:             _formatAgent,
    fail_mode_policy:  _formatFailModePolicy,
    escalation_target: _formatEscalationTarget,
  };

  // _primaryRows composes the fixed primary block — Kind / ID /
  // Label / Connected edges — in the documented order. Connected
  // edges is omitted when the carrier did not supply
  // `details._connected_edges` (production view does not emit it).
  function _primaryRows(kindLabel, id, label, connectedEdges) {
    var rows = [
      ['Kind',  kindLabel || ''],
      ['ID',    id || ''],
      ['Label', label || ''],
    ];
    if (connectedEdges != null) {
      rows.push(['Connected edges', String(connectedEdges)]);
    }
    return rows;
  }

  // _renderFieldRowsHtml renders an array of [key, value] pairs
  // as the existing `.gmap-details-row` markup. Empty values fall
  // back to `—` so the row stays visually balanced.
  function _renderFieldRowsHtml(rows) {
    if (!Array.isArray(rows) || !rows.length) return '';
    var html = '';
    for (var i = 0; i < rows.length; i++) {
      var pair = rows[i];
      var k = pair[0];
      var v = pair[1];
      var raw = (v == null || v === '') ? '—' : String(v);
      html +=
        '<div class="gmap-details-row">' +
          '<span class="gmap-details-key">' + _escHtml(String(k)) + '</span>' +
          '<span class="gmap-details-val">' + _escHtml(raw) + '</span>' +
        '</div>';
    }
    return html;
  }

  // _renderTechnicalDetailsHtml wraps the technical rows in a
  // native `<details>` element so the section is collapsed by
  // default. Empty technical lists are omitted entirely.
  function _renderTechnicalDetailsHtml(rows) {
    // Drop rows whose value is empty so the collapsed section
    // doesn't render `—` lines for fields the projection pruned.
    var present = [];
    for (var i = 0; i < (rows || []).length; i++) {
      var pair = rows[i];
      if (pair && pair[1] !== '' && pair[1] !== null && pair[1] !== undefined) {
        present.push(pair);
      }
    }
    if (!present.length) return '';
    return (
      '<details class="gmap-details-technical">' +
        '<summary>Technical details</summary>' +
        '<div class="gmap-details-technical-body">' +
          _renderFieldRowsHtml(present) +
        '</div>' +
      '</details>'
    );
  }

  // ── Value coercers ────────────────────────────────────────────────────
  function _bool(v) {
    if (v === true)  return 'true';
    if (v === false) return 'false';
    return '';
  }
  function _list(v) {
    if (!Array.isArray(v) || v.length === 0) return '';
    return v.join(', ');
  }
  function _obj(v) {
    if (!v || typeof v !== 'object') return '';
    try { return JSON.stringify(v); } catch (_) { return String(v); }
  }

  // ── Drive the inspector rail ──────────────────────────────────────────
  //
  // selectNode is the user-facing entry point. The Authority view's
  // addNode stores _kind + _id in dataset.nodeDetails (JSON); we read
  // them back to pick the formatter.
  function selectNode(nodeId) {
    var sel = _selection();
    if (typeof sel.setSelected === 'function') sel.setSelected(nodeId);

    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var nodes = canvas.querySelectorAll('.gmap-node');
    var selectedNode = null;
    nodes.forEach(function (n) {
      var isSel = n.dataset.nodeId === nodeId;
      n.classList.toggle('selected', isSel);
      if (isSel) selectedNode = n;
    });
    if (!selectedNode) return;
    _renderInto(selectedNode);

    // D32h-fix-2b — Notify the evidence-tray selection hook so the
    // bottom workbench refreshes on Authority-node clicks. Mirrors
    // Context's notify at context-graph-inspector.js:111-113. The
    // _inspectorHooks bag (index.html:1864-1878) fans out to BOTH
    // contextEvidenceTray.notifySelectionChanged and
    // authorityWorkbench.notifySelectionChanged — each gates on the
    // active lens internally, so the call is safe under either lens.
    var hooks = (window.MIDASExplorerGraph && window.MIDASExplorerGraph._inspectorHooks) || {};
    if (typeof hooks.notifyEvidenceTraySelectionChanged === 'function') {
      hooks.notifyEvidenceTraySelectionChanged();
    }
  }

  function _renderInto(selectedNode) {
    var details = {};
    try { details = JSON.parse(selectedNode.dataset.nodeDetails || '{}'); } catch (_) { /* ignore */ }
    var projKind = details._kind || '';
    var projId   = details._id   || selectedNode.dataset.nodeId || '';
    var projLabel = details._label || selectedNode.dataset.nodeName || projId;
    var adapter = _adapter();
    var kindLabel = adapter ? adapter.nodeKindLabel(projKind) : projKind;

    var insp = _inspector();
    if (typeof insp.setName === 'function') {
      insp.setName(projLabel || projId);
    }

    // D33a-spike-2g-impl-5e — Three-block selected-node content
    // model. The Authority inspector composes its own field HTML
    // (primary block + node-specific block + collapsed technical
    // details) and writes it directly to `#gmap-details-fields`,
    // bypassing the shared `inspector.setFields` uniform-row
    // renderer. The shared inspector helper is still used for
    // setName / setSummary / setGovernance — only the field-rows
    // slot is locally composed.
    var primary = _primaryRows(kindLabel, projId, projLabel, details && details._connected_edges);
    var formatter = FORMATTERS[projKind];
    var formatted = formatter ? formatter(details, { id: projId, label: projLabel }) : { specific: [], technical: [] };
    var specificRows = (formatted && formatted.specific) ? formatted.specific.filter(function (r) {
      return r && r[1] !== '' && r[1] !== null && r[1] !== undefined;
    }) : [];
    var technicalRows = (formatted && formatted.technical) ? formatted.technical : [];

    var fieldsEl = document.getElementById('gmap-details-fields');
    if (fieldsEl) {
      fieldsEl.innerHTML =
        _renderFieldRowsHtml(primary) +
        _renderFieldRowsHtml(specificRows) +
        _renderTechnicalDetailsHtml(technicalRows);
    } else if (typeof insp.setFields === 'function') {
      // Defensive fallback for test isolation / partial asset
      // loads: route the visible rows through the shared helper
      // so the inspector still renders something. Technical
      // details are dropped in this path because the shared
      // renderer doesn't support nested HTML.
      insp.setFields(primary.concat(specificRows));
    }

    // D33a-spike-2g-impl-5f — Inspector ownership cleanup. The
    // Authority Inspector tab now owns ONLY the selected object's
    // identity + direct attributes. Per-node diagnostics and
    // surface posture (previously injected into the shared
    // `#gmap-details-governance` slot via `_buildOverlayHTML`) are
    // owned by the Diagnostics and Posture & Help tabs, rendered
    // by `authority-diagnostics-panel.js` and
    // `authority-surface-posture-panel.js` respectively.
    //
    // setSummary/setGovernance are still called so that switching
    // selections (or lenses) clears any prior content the shared
    // helpers might have written. Context Graph continues to use
    // both slots — `setSummary([])` here only clears the slot when
    // the Authority lens is active; Context's own inspector path
    // re-populates them when Context is active.
    if (typeof insp.setSummary === 'function') {
      insp.setSummary([]);
    }
    if (typeof insp.setGovernance === 'function') {
      insp.setGovernance('');
    }
    if (typeof insp.setActions === 'function') {
      insp.setActions([]);
    }
    if (typeof insp.setInlineActions === 'function') {
      insp.setInlineActions(selectedNode, []);
    }
  }

  // renderNode — lens dispatch entry point. graph-inspector.render
  // resolves the mount and calls this with the selected DOM node.
  function renderNode(node, mount) {
    if (!node) return;
    _renderInto(node);
    void mount;
  }

  function clear(mount) {
    var insp = _inspector();
    if (typeof insp.setName       === 'function') insp.setName('');
    if (typeof insp.setFields     === 'function') insp.setFields([]);
    if (typeof insp.setSummary    === 'function') insp.setSummary([]);
    if (typeof insp.setGovernance === 'function') insp.setGovernance('');
    if (typeof insp.setActions    === 'function') insp.setActions([]);
    void mount;
  }

  // ── Register with the lens-agnostic inspector dispatch ───────────────
  var inspectorImpl = {
    renderNode: renderNode,
    clear:      clear,
  };

  if (window.MIDASExplorerGraph.inspector && typeof window.MIDASExplorerGraph.inspector.register === 'function') {
    window.MIDASExplorerGraph.inspector.register('authority', inspectorImpl);
  }

  window.MIDASExplorerGraph.authorityInspector = {
    selectNode: selectNode,
    renderNode: renderNode,
    clear:      clear,
    FORMATTERS: FORMATTERS,
  };
})();
