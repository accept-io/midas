// /explorer/assets/js/graph/authority/authority-graph-inspector.js — D32b-impl-1
//
// Authority Graph lens inspector. Sibling to context-graph-inspector.js;
// production owner of Authority-specific inspector content.
//
// Provides basic card-level formatting for every Authority node kind
// using typed-data already present in the graph response. No extra
// entity-detail APIs are called in this tranche — the inspector
// shows what the graph already gives us. Diagnostics / surface posture
// per-node rendering is deferred to D32b-impl-2.
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

  function _formatBusinessService(d, n) {
    return [
      ['id',                   n.id || ''],
      ['name',                 d.name || ''],
      ['status',               d.status || ''],
      ['owner',                d.owner || ''],
      ['service_type',         d.service_type || ''],
      ['fail_mode_policy_id',  d.fail_mode_policy_id || '—'],
    ];
  }

  function _formatDecisionSurface(d, n) {
    return [
      ['id',                       n.id || ''],
      ['version',                  d.version != null ? String(d.version) : ''],
      ['name',                     d.name || ''],
      ['status',                   d.status || ''],
      ['process_id',               d.process_id || ''],
      ['business_service_id',      d.business_service_id || ''],
      ['effective_policy_source',  d.effective_policy_source || ''],
      ['effective_policy_id',      d.effective_policy_id || '—'],
      ['inherits_bs_policy',       _bool(d.inherits_bs_policy)],
    ];
  }

  function _formatAuthorityProfile(d, n) {
    return [
      ['id',                    n.id || ''],
      ['version',               d.version != null ? String(d.version) : ''],
      ['surface_id',            d.surface_id || ''],
      ['name',                  d.name || ''],
      ['status',                d.status || ''],
      ['validity_status',       d.validity_status || ''],
      ['confidence_threshold',  d.confidence_threshold != null ? String(d.confidence_threshold) : ''],
      ['consequence_threshold', d.consequence_threshold != null ? String(d.consequence_threshold) : ''],
      ['escalation_mode',       d.escalation_mode || ''],
      ['escalation_target_id',  d.escalation_target_id || '—'],
      ['fail_mode',             d.fail_mode || ''],
    ];
  }

  function _formatAuthorityGrant(d, n) {
    return [
      ['id',              n.id || ''],
      ['profile_id',      d.profile_id || ''],
      ['agent_id',        d.agent_id || ''],
      ['status',          d.status || ''],
      ['validity_status', d.validity_status || ''],
      ['capabilities',    _list(d.capabilities)],
      ['constraints',     _obj(d.constraints)],
    ];
  }

  function _formatAgent(d, n) {
    return [
      ['id',                n.id || ''],
      ['name',              d.name || ''],
      ['type',              d.type || ''],
      ['owner',             d.owner || ''],
      ['model_version',     d.model_version || ''],
      ['operational_state', d.operational_state || ''],
    ];
  }

  function _formatFailModePolicy(d, n) {
    return [
      ['id',                   n.id || ''],
      ['version',              d.version != null ? String(d.version) : ''],
      ['name',                 d.name || ''],
      ['status',               d.status || ''],
      ['effective_date',       d.effective_date || ''],
      ['effective_until',      d.effective_until || ''],
      ['business_owner',       d.business_owner || ''],
      ['technical_owner',      d.technical_owner || ''],
      ['origin',               d.origin || ''],
      ['managed',              _bool(d.managed)],
      ['rule_count_by_class',  _obj(d.rule_count_by_class)],
    ];
  }

  function _formatEscalationTarget(d, n) {
    return [
      ['id',               n.id || ''],
      ['version',          d.version != null ? String(d.version) : ''],
      ['name',             d.name || ''],
      ['kind',             d.kind || ''],
      ['handle',           d.handle || ''],
      ['status',           d.status || ''],
      ['effective_date',   d.effective_date || ''],
      ['effective_until',  d.effective_until || ''],
      ['business_owner',   d.business_owner || ''],
      ['technical_owner',  d.technical_owner || ''],
    ];
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
    var formatter = FORMATTERS[projKind];
    var rows = formatter ? formatter(details, { id: projId, label: projLabel }) : [];
    if (typeof insp.setFields === 'function') {
      // Filter empty values so the inspector card stays compact when
      // the projection prunes a field. The contract requires fields
      // be SUPPORTED (formatter handles the kind); the rail only
      // shows rows where the value is non-empty.
      insp.setFields(rows.filter(function (r) { return r[1] !== '' && r[1] !== null && r[1] !== undefined; }));
    }
    if (typeof insp.setSummary === 'function') {
      insp.setSummary([['Kind', kindLabel]]);
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
