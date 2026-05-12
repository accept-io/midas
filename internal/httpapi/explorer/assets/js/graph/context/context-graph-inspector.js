// /explorer/assets/js/graph/context/context-graph-inspector.js — D32a-impl-4
//
// Context lens inspector. Production owner of Context-Graph-specific
// inspector content:
//   • selectNode(nodeId) — drive the inspector rail from a node id
//   • the fail-mode-policy reference block (business / surface)
//   • the GOVERNANCE_KEYS filter that keeps fail_mode_policy_id from
//     surfacing in the generic field rows.
//
// The lens-agnostic inspector module (graph-inspector.js) provides
// frame setters; this module composes those into the full per-node
// inspector content for the Context lens.
//
// External dependencies:
//   window.MIDASExplorerGraph.inspector.setName/setFields/setSummary/
//     setGovernance/setActions/setInlineActions
//   window.MIDASExplorerGraph.selection.setSelected
//   window.MIDASExplorerGraph._inspectorHooks (inline-callbacks for
//     evidence tray + selected gmapData)
//   window.MIDASExplorerUtils.escHtml
//
// Public surface on window.MIDASExplorerGraph.contextInspector:
//   selectNode(nodeId) — orchestrate selection + render the rail
//   buildFailModePolicySection(nodeKind, details, data) — pure HTML
//     builder for the Governance section (Context-specific).

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _inspector()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.inspector) || {}; }
  function _selection()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.selection) || {}; }
  function _hooks()      { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._inspectorHooks) || {}; }

  // GOVERNANCE_KEYS — node-details keys that are surfaced by the
  // dedicated Governance section rather than the generic field rows.
  // Context-specific (the Context Graph adapter populates these
  // fields on business_service and decision_surface details).
  var GOVERNANCE_KEYS = ['fail_mode_policy_id'];

  // buildFailModePolicySection renders Context Graph reference-level
  // governance content. Pure function — no DOM, no fetch. Wording is
  // restricted to the Context Graph adapter's allowed vocabulary
  // (Default policy / Surface override / Inherited default /
  // Effective source / Business service default / Evidence only /
  // Soft/open / Not enabled / None configured).
  function buildFailModePolicySection(nodeKind, details, data) {
    if (!details || typeof details !== 'object') return '';
    var ownPolicy = String(details.fail_mode_policy_id || '');
    var bsPolicy = (data && data.business_service && data.business_service.fail_mode_policy_id)
      ? String(data.business_service.fail_mode_policy_id) : '';
    var rows = [];
    if (nodeKind === 'business') {
      if (ownPolicy) {
        rows.push(['Default policy', '<code class="gmap-fmp-reference-code">' + _escHtml(ownPolicy) + '</code>']);
        rows.push(['Source', 'Business service default']);
      } else {
        rows.push(['Default policy', 'None configured']);
      }
    } else if (nodeKind === 'surface') {
      if (ownPolicy) {
        rows.push(['Surface override', '<code class="gmap-fmp-reference-code">' + _escHtml(ownPolicy) + '</code>']);
        rows.push(['Effective source', 'Surface override']);
      } else if (bsPolicy) {
        rows.push(['Surface override', 'None']);
        rows.push(['Inherited default', '<code class="gmap-fmp-reference-code">' + _escHtml(bsPolicy) + '</code>']);
        rows.push(['Effective source', 'Business service default']);
      } else {
        rows.push(['Surface override', 'None']);
        rows.push(['Inherited default', 'None configured']);
      }
    } else {
      return '';
    }
    rows.push(['Runtime effect', 'Evidence only']);
    rows.push(['Soft/open', 'Not enabled']);
    var rowsHtml = rows.map(function (pair) {
      var k = pair[0], v = pair[1];
      return '<div class="gmap-fmp-reference-row">' +
        '<span class="gmap-fmp-reference-key">' + _escHtml(String(k)) + '</span>' +
        '<span class="gmap-fmp-reference-val">' + v + '</span>' +
      '</div>';
    }).join('');
    return (
      '<div class="gmap-details-section">' +
        '<div class="gmap-details-title">Fail Mode Policy</div>' +
        '<div class="gmap-fmp-reference">' + rowsHtml + '</div>' +
      '</div>'
    );
  }

  // selectNode — drive the inspector rail from a Context Graph node
  // id. Reads node DOM dataset (set by the renderer's addNode), pushes
  // selection through graph-selection, and composes the rail's name,
  // generic fields, governance section, and bottom-panel actions
  // through the lens-agnostic inspector frame.
  function selectNode(nodeId) {
    var sel = _selection();
    if (typeof sel.setSelected === 'function') sel.setSelected(nodeId);

    // Evidence-tray notification (still owned inline; called through
    // the inspector-hook bundle so the module is not coupled to the
    // tray implementation).
    var h = _hooks();
    if (typeof h.notifyEvidenceTraySelectionChanged === 'function') {
      h.notifyEvidenceTraySelectionChanged();
    }

    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var nodes = canvas.querySelectorAll('.gmap-node');
    var selectedNode = null;
    nodes.forEach(function (n) {
      var isSel = n.dataset.nodeId === nodeId;
      n.classList.toggle('selected', isSel);
      if (isSel) selectedNode = n;
      if (!isSel) {
        var stale = n.querySelector('.gmap-node-inline-actions');
        if (stale) {
          stale.innerHTML = '';
          stale.setAttribute('hidden', '');
        }
      }
    });
    if (!selectedNode) return;
    var insp = _inspector();
    if (typeof insp.setName === 'function') insp.setName(selectedNode.dataset.nodeName || nodeId);
    var details = {};
    try { details = JSON.parse(selectedNode.dataset.nodeDetails || '{}'); } catch (_) { /* ignore */ }
    if (typeof insp.setFields === 'function') {
      insp.setFields(
        Object.keys(details)
          .filter(function (k) { return GOVERNANCE_KEYS.indexOf(k) < 0; })
          .map(function (k) { return [k, details[k]]; })
      );
    }
    var data = (typeof h.getGmapData === 'function') ? h.getGmapData() : null;
    if (typeof insp.setGovernance === 'function') {
      insp.setGovernance(buildFailModePolicySection(selectedNode.dataset.nodeKind || '', details, data));
    }
    var actions = [];
    try { actions = JSON.parse(selectedNode.dataset.nodeActions || '[]'); } catch (_) { /* ignore */ }
    if (typeof insp.setActions === 'function') {
      insp.setActions(Array.isArray(actions) ? actions : []);
    }
    if (typeof insp.setInlineActions === 'function') {
      insp.setInlineActions(selectedNode, Array.isArray(actions) ? actions : []);
    }
  }

  window.MIDASExplorerGraph.contextInspector = {
    selectNode:                 selectNode,
    buildFailModePolicySection: buildFailModePolicySection,
    GOVERNANCE_KEYS:            GOVERNANCE_KEYS,
  };
})();
