// /explorer/assets/js/graph/authority/authority-graph-workbench.js
//   — D32h-fix-1
//
// Authority-lens bottom workbench. Sibling of the Context lens's
// #gmap-evidence-tray (Drift Analytics). Lives under the same
// .governance-map-workbench parent so the workbench column reserves
// the same vertical space regardless of lens; the parent shell never
// has to know which lens is active.
//
// Lens routing is delegated to CSS. body[data-graph-lens="authority"]
// hides #gmap-evidence-tray and reveals #gmap-authority-workbench;
// body[data-graph-lens="context"] does the opposite. The router that
// sets body.dataset.graphLens is the existing inline workbench code
// (index.html — predates this tranche); D32h-fix-1 does not change
// it.
//
// This module:
//   • renders projection-derived content into the workbench's panel
//     mount on every Authority paint;
//   • supports five tabs — Overview / Fail Mode / Escalation /
//     Grants / Evidence — defined by data-authority-tab buttons in
//     index.html;
//   • re-renders on operator-driven node selection (subscribes to the
//     existing _gmapRenderCtx selectNode hook via a small observer);
//   • renders an empty / placeholder state when no Authority data is
//     cached.
//
// Constraints:
//   • Never fetches or otherwise calls the backend. All data comes
//     from window.MIDASExplorerGraph._lastAuthorityProjection (the
//     spec cache set by the Authority view).
//   • Never touches #gmap-evidence-tray or any Context module.
//   • Never invents runtime evidence counters. The Evidence tab is
//     intentionally a placeholder.
//
// Public surface (window.MIDASExplorerGraph.authorityWorkbench):
//
//   init()                — wire tabs + toggle. Idempotent.
//   render()              — repaint the active tab from the cached
//                           projection. Cheap; called every Authority
//                           paint via the view's post-paint hook.
//   setActiveTab(id)      — programmatically switch tab.
//   notifySelectionChanged(refKey)
//                         — repaint the active tab when the operator
//                           selects a different node.
//   _TAB_IDS              — test-visible constant.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var TAB_IDS = Object.freeze(['overview', 'fail-mode', 'escalation', 'grants', 'evidence']);

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _store() { return window.MIDASExplorerStore || null; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _isAuthorityLens() {
    var s = _store();
    if (!s || typeof s.getState !== 'function') return false;
    try { return s.getState().selectedGraphLens === 'authority'; }
    catch (_) { return false; }
  }
  function _spec() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._lastAuthorityProjection) || null;
  }
  function _selectedRefKey() {
    var st = window.MIDASExplorerGraph && window.MIDASExplorerGraph.state;
    return (st && typeof st.selectedId === 'string') ? st.selectedId : '';
  }

  // ── DOM accessors ──────────────────────────────────────────────────
  function _root()        { return document.getElementById('gmap-authority-workbench'); }
  function _toggleBtn()   { return document.getElementById('gmap-authority-workbench-toggle'); }
  function _panelMount()  { return document.getElementById('gmap-authority-workbench-panel'); }
  function _subtitleEl()  { return document.querySelector('[data-authority-workbench-subtitle]'); }
  function _tabButtons()  { return document.querySelectorAll('[data-authority-tab]'); }

  // ── State ──────────────────────────────────────────────────────────
  var _activeTab = 'overview';
  var _expanded  = false;
  var _initialised = false;

  function _setExpanded(expanded) {
    _expanded = !!expanded;
    var root = _root();
    if (!root) return;
    root.classList.toggle('is-expanded', _expanded);
    var btn = _toggleBtn();
    if (btn) {
      btn.setAttribute('aria-expanded', _expanded ? 'true' : 'false');
      btn.setAttribute('aria-label', _expanded ? 'Collapse Authority Workbench' : 'Expand Authority Workbench');
      btn.title = _expanded ? 'Collapse Authority Workbench' : 'Expand Authority Workbench';
      // D32h-fix-2d-converge — Flip the toggle glyph so the operator
      // sees the affordance state. Mirrors Context tray-toggle behaviour
      // at context-evidence-tray.js:1016-1017. Without this, the
      // collapse control reads as visually static — the operator
      // perceives it as "no close control" (operator-reported defect
      // #1 in the parity analysis).
      var glyph = btn.querySelector('span[aria-hidden]');
      if (glyph) {
        glyph.textContent = _expanded ? '▼' : '▲';
      }
    }
    // D32h-fix-2d-converge — Schedule a re-fit after the 0.18s height
    // transition completes so the graph repositions to the new
    // canvas-scroll height. Mirrors Context tray's pattern at
    // context-evidence-tray.js:1039-1043 (200ms timeout). Without
    // this, expanding the workbench shrinks the canvas-scroll
    // viewport and the graph paints clipped nodes / leaves dead
    // space until the next manual fit.
    if (typeof window.setTimeout === 'function') {
      window.setTimeout(function () {
        var camera = window.MIDASExplorerGraph && window.MIDASExplorerGraph.camera;
        if (camera && typeof camera.scheduleFitToView === 'function') {
          try { camera.scheduleFitToView(); } catch (_) { /* swallow */ }
        }
      }, 200);
    }
  }

  function setActiveTab(id) {
    if (TAB_IDS.indexOf(id) < 0) return;
    _activeTab = id;
    var btns = _tabButtons();
    for (var i = 0; i < btns.length; i++) {
      var b = btns[i];
      var on = b.getAttribute('data-authority-tab') === id;
      b.classList.toggle('is-active', on);
      b.setAttribute('aria-selected', on ? 'true' : 'false');
    }
    render();
  }

  // ── Lookup helpers (projection-derived) ───────────────────────────
  function _selectedNode() {
    var spec = _spec();
    if (!spec) return null;
    var key = _selectedRefKey();
    if (!key) return spec.root || null;
    var byRef = spec.nodesByRef || {};
    return byRef[key] || spec.root || null;
  }
  function _chainContainingSurface(spec, surfaceId) {
    var chains = (spec && spec.chains) || [];
    for (var i = 0; i < chains.length; i++) {
      var c = chains[i];
      if (c && c.surface && c.surface.id === surfaceId) return c;
    }
    return null;
  }
  function _chainContainingProfile(spec, profileId) {
    var chains = (spec && spec.chains) || [];
    for (var i = 0; i < chains.length; i++) {
      var c = chains[i];
      if (c && c.profile && c.profile.id === profileId) return c;
    }
    return null;
  }
  function _chainContainingGrant(spec, grantId) {
    var chains = (spec && spec.chains) || [];
    for (var i = 0; i < chains.length; i++) {
      var c = chains[i];
      if (c && c.grant && c.grant.id === grantId) return c;
    }
    return null;
  }
  function _chainContainingAgent(spec, agentId) {
    var chains = (spec && spec.chains) || [];
    for (var i = 0; i < chains.length; i++) {
      var c = chains[i];
      if (c && c.agent && c.agent.id === agentId) return c;
    }
    return null;
  }

  function _fmpForOwnerRef(spec, ownerKind, ownerId) {
    var fmps = ((spec && spec.governance) || {}).failModePolicies || [];
    for (var i = 0; i < fmps.length; i++) {
      var f = fmps[i];
      var owners = (f && f.owners) || [];
      for (var j = 0; j < owners.length; j++) {
        if (owners[j].kind === ownerKind && owners[j].id === ownerId) return f;
      }
    }
    return null;
  }
  function _etForProfile(spec, profileId) {
    var ets = ((spec && spec.governance) || {}).escalationTargets || [];
    for (var i = 0; i < ets.length; i++) {
      var e = ets[i];
      var owners = (e && e.owners) || [];
      for (var j = 0; j < owners.length; j++) {
        if (owners[j].kind === 'authority_profile' && owners[j].id === profileId) return e;
      }
    }
    return null;
  }

  function _postureFor(spec, surfaceId) {
    var arr = (spec && spec.surfacePosture) || [];
    for (var i = 0; i < arr.length; i++) {
      var p = arr[i];
      if (p && p.surface && p.surface.id === surfaceId) return p;
    }
    return null;
  }

  // ── Subtitle reflection ───────────────────────────────────────────
  function _renderSubtitle(node) {
    var el = _subtitleEl();
    if (!el) return;
    if (!node) {
      el.textContent = 'Select a graph node to inspect authority posture.';
      return;
    }
    var label = node.label || node.id || '(unnamed)';
    var kind  = node.kind  || '';
    if (kind) {
      el.textContent = kind.replace(/_/g, ' ') + ' — ' + label;
    } else {
      el.textContent = String(label);
    }
  }

  // ── Tab renderers ─────────────────────────────────────────────────
  function _statRow(label, value, modifier) {
    var cls = 'authority-workbench-stat' + (modifier ? ' ' + modifier : '');
    return (
      '<div class="' + cls + '">' +
        '<span class="authority-workbench-stat-label">' + _escHtml(label) + '</span>' +
        '<span class="authority-workbench-stat-value">' + _escHtml(String(value)) + '</span>' +
      '</div>'
    );
  }

  function _renderOverview() {
    var spec = _spec();
    if (!spec) return _emptyState('No Authority projection loaded.');
    var summary = spec.summary || {};
    var diagSum = spec.diagnosticSummary || {};
    var chainCount   = (spec.chains || []).length;
    var fmpCount     = ((spec.governance || {}).failModePolicies  || []).length;
    var etCount      = ((spec.governance || {}).escalationTargets || []).length;
    var unlinkedN    = (spec.unlinked || []).length;

    var stats = '';
    stats += _statRow('Decision surfaces', chainCount);
    stats += _statRow('Active profiles',   summary.active_profile_count || 0);
    stats += _statRow('Active grants',     summary.active_grant_count || 0);
    stats += _statRow('Active agents',     summary.active_agent_count || 0);
    if ((summary.grants_with_stop_capability || 0) > 0) {
      stats += _statRow('Stop-capability grants', summary.grants_with_stop_capability, 'authority-workbench-stat-emphasis');
    }
    stats += _statRow('Fail-mode policies in scope',  fmpCount);
    stats += _statRow('Escalation targets in scope',  etCount);
    if (unlinkedN > 0) {
      stats += _statRow('Unlinked / orphan nodes', unlinkedN, 'authority-workbench-stat-gap');
    }

    var gaps = '';
    function gapRow(label, list) {
      if (!Array.isArray(list) || list.length === 0) return '';
      return _statRow(label, list.length, 'authority-workbench-stat-gap');
    }
    gaps += gapRow('Surfaces missing profile',           summary.surfaces_without_profiles);
    gaps += gapRow('Profiles without grants',            summary.profiles_without_grants);
    gaps += gapRow('Grants missing agent',               summary.grants_without_agents);
    gaps += gapRow('Surfaces without fail-mode policy',  summary.surfaces_without_effective_fail_mode_policy);
    gaps += gapRow('Policies missing active version',    summary.policies_missing_active_version);
    gaps += gapRow('Profiles with dangling escalation',  summary.profiles_with_dangling_escalation_target);

    var diag = '';
    if ((diagSum.critical || 0) > 0) diag += _statRow('Critical diagnostics', diagSum.critical, 'authority-workbench-stat-critical');
    if ((diagSum.warning  || 0) > 0) diag += _statRow('Warning diagnostics',  diagSum.warning,  'authority-workbench-stat-warning');
    if ((diagSum.info     || 0) > 0) diag += _statRow('Info diagnostics',     diagSum.info,     'authority-workbench-stat-info');

    return (
      '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Authority posture</h4>' +
        '<div class="authority-workbench-stats">' + stats + '</div>' +
      '</div>' +
      (gaps ? '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Coverage gaps</h4>' +
        '<div class="authority-workbench-stats">' + gaps + '</div>' +
      '</div>' : '') +
      (diag ? '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Diagnostics</h4>' +
        '<div class="authority-workbench-stats">' + diag + '</div>' +
      '</div>' : '')
    );
  }

  function _renderFailMode() {
    var spec = _spec();
    if (!spec) return _emptyState('No Authority projection loaded.');
    var node = _selectedNode();
    if (!node) return _emptyState('Select a business service or decision surface to inspect fail-mode posture.');

    // Locate the FMP relevant to the selected node.
    var fmpSpec = null;
    var source  = '';
    if (node.kind === 'business_service') {
      fmpSpec = _fmpForOwnerRef(spec, 'business_service', node.id);
      source  = fmpSpec ? 'business_service_default' : 'missing';
    } else if (node.kind === 'decision_surface') {
      fmpSpec = _fmpForOwnerRef(spec, 'decision_surface', node.id);
      source  = fmpSpec ? 'surface_override' : (spec.root && _fmpForOwnerRef(spec, 'business_service', spec.root.id) ? 'business_service_default' : 'missing');
      if (source === 'business_service_default' && !fmpSpec) {
        fmpSpec = _fmpForOwnerRef(spec, 'business_service', spec.root.id);
      }
    } else {
      return _emptyState('Select a business service or decision surface to inspect fail-mode posture.');
    }

    var rows = '';
    rows += _statRow('Policy source', source.replace(/_/g, ' '));
    if (fmpSpec && fmpSpec.node) {
      rows += _statRow('Policy id',   fmpSpec.node.id || '—');
      rows += _statRow('Policy name', fmpSpec.node.label || fmpSpec.node.id || '—');
      var fmpData = fmpSpec.node.fail_mode_policy || {};
      if (fmpData.origin)         rows += _statRow('Origin',  fmpData.origin);
      if (fmpData.version != null) rows += _statRow('Version', 'v' + fmpData.version);
      if (fmpData.status)         rows += _statRow('Status',  fmpData.status);
      var rcb = fmpData.rule_count_by_class || null;
      if (rcb && typeof rcb === 'object') {
        var classes = Object.keys(rcb).sort();
        for (var ci = 0; ci < classes.length; ci++) {
          rows += _statRow('Rules — ' + classes[ci], rcb[classes[ci]]);
        }
      }
    } else {
      rows += _statRow('Policy', 'Missing', 'authority-workbench-stat-gap');
    }

    // Posture details for a surface selection.
    var postureBlock = '';
    if (node.kind === 'decision_surface') {
      var p = _postureFor(spec, node.id);
      if (p) {
        var pr = '';
        if (p.fail_mode_policy_status) pr += _statRow('FMP status',    p.fail_mode_policy_status);
        if (p.profile_status)          pr += _statRow('Profile',       p.profile_status);
        if (p.grant_status)            pr += _statRow('Grant',         p.grant_status);
        if (p.agent_status)            pr += _statRow('Agent',         p.agent_status);
        if (p.authority_status)        pr += _statRow('Authority',     p.authority_status);
        if (p.escalation_status)       pr += _statRow('Escalation',    p.escalation_status);
        if (pr) postureBlock = (
          '<div class="authority-workbench-section">' +
            '<h4 class="authority-workbench-section-title">Surface posture</h4>' +
            '<div class="authority-workbench-stats">' + pr + '</div>' +
          '</div>'
        );
      }
    }

    return (
      '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Fail-mode policy</h4>' +
        '<div class="authority-workbench-stats">' + rows + '</div>' +
      '</div>' +
      postureBlock
    );
  }

  function _renderEscalation() {
    var spec = _spec();
    if (!spec) return _emptyState('No Authority projection loaded.');
    var node = _selectedNode();
    if (!node) return _emptyState('Select an authority profile or decision surface to inspect escalation.');

    var profileNode = null;
    var contextLabel = '';
    if (node.kind === 'authority_profile') {
      profileNode = node;
      contextLabel = 'Authority profile';
    } else if (node.kind === 'decision_surface') {
      var ch = _chainContainingSurface(spec, node.id);
      profileNode = ch && ch.profile;
      contextLabel = profileNode ? 'Profile attached to selected surface' : '';
    }

    if (!profileNode) {
      return _emptyState('Select an authority profile or a surface that uses one.');
    }

    var pd = profileNode.authority_profile || {};
    var rows = '';
    rows += _statRow('Profile',           profileNode.label || profileNode.id);
    rows += _statRow('Escalation mode',   pd.escalation_mode || '—');
    if (pd.confidence_threshold != null)   rows += _statRow('Confidence threshold',   pd.confidence_threshold);
    if (pd.consequence_threshold != null)  rows += _statRow('Consequence threshold',  pd.consequence_threshold);
    if (pd.escalation_target_id)           rows += _statRow('Escalation target id',   pd.escalation_target_id);

    var etBlock = '';
    var etSpec = _etForProfile(spec, profileNode.id);
    if (etSpec && etSpec.node) {
      var ed = etSpec.node.escalation_target || {};
      var erows = '';
      erows += _statRow('Target',  etSpec.node.label || etSpec.node.id);
      if (ed.kind)             erows += _statRow('Kind',    ed.kind);
      if (ed.version != null)  erows += _statRow('Version', 'v' + ed.version);
      if (etSpec.shared)       erows += _statRow('Shared by profiles', (etSpec.owners || []).length, 'authority-workbench-stat-emphasis');
      etBlock = (
        '<div class="authority-workbench-section">' +
          '<h4 class="authority-workbench-section-title">Escalation target</h4>' +
          '<div class="authority-workbench-stats">' + erows + '</div>' +
        '</div>'
      );
    } else {
      etBlock = (
        '<div class="authority-workbench-section">' +
          '<h4 class="authority-workbench-section-title">Escalation target</h4>' +
          '<div class="authority-workbench-empty">No escalation target attached.</div>' +
        '</div>'
      );
    }

    return (
      (contextLabel ? '<div class="authority-workbench-section-context">' + _escHtml(contextLabel) + '</div>' : '') +
      '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Escalation</h4>' +
        '<div class="authority-workbench-stats">' + rows + '</div>' +
      '</div>' +
      etBlock
    );
  }

  function _renderGrants() {
    var spec = _spec();
    if (!spec) return _emptyState('No Authority projection loaded.');
    var node = _selectedNode();
    if (!node) return _emptyState('Select a surface, profile, grant, or agent to inspect authorisation.');

    var ch = null;
    if (node.kind === 'decision_surface')  ch = _chainContainingSurface(spec, node.id);
    else if (node.kind === 'authority_profile') ch = _chainContainingProfile(spec, node.id);
    else if (node.kind === 'authority_grant')   ch = _chainContainingGrant(spec, node.id);
    else if (node.kind === 'agent')             ch = _chainContainingAgent(spec, node.id);

    if (!ch) {
      return _emptyState('Selected node is not part of a chain.');
    }

    var rows = '';
    if (ch.surface)  rows += _statRow('Surface',  ch.surface.label || ch.surface.id);
    if (ch.profile)  rows += _statRow('Profile',  ch.profile.label || ch.profile.id);
    else             rows += _statRow('Profile',  'Missing', 'authority-workbench-stat-gap');
    if (ch.grant)    rows += _statRow('Grant',    ch.grant.label || ch.grant.id);
    else             rows += _statRow('Grant',    'Missing', 'authority-workbench-stat-gap');
    if (ch.agent)    rows += _statRow('Agent',    ch.agent.label || ch.agent.id);
    else             rows += _statRow('Agent',    'Missing', 'authority-workbench-stat-gap');

    var grantBlock = '';
    if (ch.grant) {
      var gd = ch.grant.authority_grant || {};
      var grows = '';
      if (gd.status)            grows += _statRow('Status',         gd.status);
      if (gd.validity_status)   grows += _statRow('Validity',       gd.validity_status);
      var caps = gd.capabilities || gd.authority_capabilities || [];
      if (Array.isArray(caps) && caps.length > 0) {
        for (var ci = 0; ci < caps.length; ci++) {
          var c = caps[ci];
          var capLabel = (typeof c === 'string') ? c : (c && c.kind) || '';
          if (!capLabel) continue;
          var emphasis = (capLabel === 'stop') ? 'authority-workbench-stat-emphasis' : '';
          grows += _statRow('Capability', capLabel, emphasis);
        }
      }
      var constraints = gd.constraints || [];
      if (Array.isArray(constraints) && constraints.length > 0) {
        grows += _statRow('Constraints', constraints.length);
      }
      if (grows) grantBlock = (
        '<div class="authority-workbench-section">' +
          '<h4 class="authority-workbench-section-title">Grant detail</h4>' +
          '<div class="authority-workbench-stats">' + grows + '</div>' +
        '</div>'
      );
    }

    var agentBlock = '';
    if (ch.agent) {
      var ad = ch.agent.agent || {};
      var arows = '';
      if (ad.type)              arows += _statRow('Type',             ad.type);
      if (ad.model_version)     arows += _statRow('Model version',    ad.model_version);
      if (ad.operational_state) arows += _statRow('Operational state',ad.operational_state,
        (ad.operational_state === 'blocked' || ad.operational_state === 'suspended') ? 'authority-workbench-stat-gap' : '');
      if (arows) agentBlock = (
        '<div class="authority-workbench-section">' +
          '<h4 class="authority-workbench-section-title">Agent detail</h4>' +
          '<div class="authority-workbench-stats">' + arows + '</div>' +
        '</div>'
      );
    }

    return (
      '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Authority chain</h4>' +
        '<div class="authority-workbench-stats">' + rows + '</div>' +
      '</div>' +
      grantBlock +
      agentBlock
    );
  }

  function _renderEvidence() {
    // Explicit honesty: there is no runtime evidence wired through to
    // the Authority projection today. Show projection-only diagnostics
    // and a clear "not wired" marker — never fabricate counters.
    var spec = _spec();
    if (!spec) return _emptyState('No Authority projection loaded.');
    var diags = (spec.diagnostics || []).slice(0, 40);
    var rows = '';
    if (diags.length === 0) {
      rows = '<div class="authority-workbench-empty">No diagnostics in this projection.</div>';
    } else {
      rows = '<ul class="authority-workbench-diagnostics">';
      for (var i = 0; i < diags.length; i++) {
        var d = diags[i] || {};
        rows += (
          '<li class="authority-workbench-diagnostic authority-workbench-diagnostic-' + _escHtml(d.severity || 'info') + '">' +
            '<span class="authority-workbench-diagnostic-severity">' + _escHtml(d.severity || '') + '</span> ' +
            '<span class="authority-workbench-diagnostic-kind">' + _escHtml(d.kind || '') + '</span>' +
            (d.message ? ' — <span class="authority-workbench-diagnostic-message">' + _escHtml(d.message) + '</span>' : '') +
          '</li>'
        );
      }
      rows += '</ul>';
    }
    return (
      '<div class="authority-workbench-section">' +
        '<h4 class="authority-workbench-section-title">Projection diagnostics</h4>' +
        rows +
      '</div>' +
      '<div class="authority-workbench-section authority-workbench-section-note">' +
        '<h4 class="authority-workbench-section-title">Runtime evidence</h4>' +
        '<div class="authority-workbench-empty">' +
          'Runtime evidence overlay is not wired yet for the Authority lens. ' +
          'Projection-level diagnostics above are the authoritative source today.' +
        '</div>' +
      '</div>'
    );
  }

  function _emptyState(message) {
    return '<div class="authority-workbench-empty">' + _escHtml(message) + '</div>';
  }

  function render() {
    var mount = _panelMount();
    if (!mount) return;
    _renderSubtitle(_selectedNode());
    var html;
    switch (_activeTab) {
      case 'overview':    html = _renderOverview();     break;
      case 'fail-mode':   html = _renderFailMode();     break;
      case 'escalation':  html = _renderEscalation();   break;
      case 'grants':      html = _renderGrants();       break;
      case 'evidence':    html = _renderEvidence();     break;
      default:            html = _emptyState('Unknown tab.');
    }
    mount.innerHTML = html;
  }

  function notifySelectionChanged() {
    if (!_isAuthorityLens()) return;
    render();
  }

  // ── Init (idempotent) ─────────────────────────────────────────────
  function init() {
    if (_initialised) return;
    _initialised = true;
    var btns = _tabButtons();
    for (var i = 0; i < btns.length; i++) {
      btns[i].addEventListener('click', function (e) {
        var t = e.currentTarget;
        if (!t) return;
        var id = t.getAttribute('data-authority-tab');
        if (!id) return;
        setActiveTab(id);
      });
    }
    var toggle = _toggleBtn();
    if (toggle) {
      toggle.addEventListener('click', function () { _setExpanded(!_expanded); });
    }
    _setExpanded(false);
  }

  // Late-binding init: the DOM elements may not yet exist when this
  // module loads. Defer to DOMContentLoaded if needed.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  window.MIDASExplorerGraph.authorityWorkbench = {
    init:                    init,
    render:                  render,
    setActiveTab:            setActiveTab,
    notifySelectionChanged:  notifySelectionChanged,
    _TAB_IDS:                TAB_IDS,
  };
})();
