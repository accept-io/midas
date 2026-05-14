// /explorer/assets/js/governance-map/layers.js — D27j-ui-foundation-5
//
// Pure classification helpers for the Governance Map's layer/filter
// system. Both functions are pure switches: they take a string and
// return a string, with no DOM access, no fetch, no module state.
//
// Function bodies are byte-identical to the originals; only their
// physical location has moved.
//
// This file is independent of constants.js and layout.js.

(function () {
  'use strict';

  window.MIDASGovernanceMap = window.MIDASGovernanceMap || {};

  // gmapNodeCategoryFromKind maps a node card's dataset.nodeKind to
  // one of the chip categories. Two kinds collapse:
  //   - 'business' (root in service view) and 'related' (related BSes)
  //     both fall under the 'business' chip.
  //   - 'authority' and 'coverage' (synthetic right-column cards) both
  //     fall under the 'synthetic' chip.
  //
  // Returns '' for kinds that have no chip (currently only 'more',
  // the truncation indicator) — those nodes always remain visible
  // because they ARE the affordance for "more here, click to expand".
  function gmapNodeCategoryFromKind(kind) {
    switch (kind) {
      // Context lens kinds (unchanged).
      case 'business':
      case 'related':   return 'business';
      case 'cap':       return 'capability';
      case 'proc':      return 'process';
      case 'surface':   return 'surface';
      case 'ai':        return 'ai';
      case 'authority':
      case 'coverage':  return 'synthetic';
      // D32h-impl-1 — Authority lens kinds folded in. The Authority
      // adapter declares the same subject / authority / agent /
      // governance categories at authority-graph-adapter.js:119-127;
      // this classifier is the shared entry point applyVisibilityFilters
      // calls to discover a node's chip group. The Authority lens
      // namespaces its data-projection-kind attribute with the raw
      // backend kind (decision_surface / authority_profile / etc.),
      // so we map those strings here rather than re-declaring the
      // taxonomy.
      case 'decision_surface':  return 'subject';
      case 'authority_profile': return 'authority';
      case 'authority_grant':   return 'authority';
      case 'agent':             return 'agent';
      case 'fail_mode_policy':  return 'governance';
      case 'escalation_target': return 'governance';
      // business_service overlaps between lenses. Context uses
      // 'business' (the inline dataset.nodeKind = 'business'); Authority
      // emits 'business_service' as the projection kind. Map both.
      case 'business_service':  return 'subject';
      default:          return '';
    }
  }

  function gmapConnectorKindFromCls(cls) {
    const s = String(cls || '');
    // Context lens connector classes (unchanged).
    if (s.indexOf('connector-ai-binding') >= 0) return { kind: 'ai_binding',   label: 'AI binding' };
    if (s.indexOf('connector-authority')  >= 0) return { kind: 'authority',    label: 'Authority' };
    if (s.indexOf('connector-evidence')   >= 0) return { kind: 'evidence',     label: 'Evidence' };
    if (s.indexOf('connector-gap')        >= 0) return { kind: 'coverage_gap', label: 'Coverage gap' };
    if (s.indexOf('connector-service')    >= 0) return { kind: 'service',      label: 'Service relationship' };
    // D32h-impl-1 — Authority lens connector classes. Names mirror the
    // wire-level edge kinds the adapter emits (authority-graph-
    // adapter.js connectorClassForEdge). Per-edge labels supply the
    // hover tooltip text the shared renderer assembles via
    // pathEl.setAttribute('aria-label', kindInfo.label + ' from … to …').
    if (s.indexOf('authority-connector-business_service_has_surface')          >= 0) return { kind: 'authority_bs_surface',     label: 'Business service has surface' };
    if (s.indexOf('authority-connector-surface_uses_profile')                  >= 0) return { kind: 'authority_surface_profile', label: 'Surface uses profile' };
    if (s.indexOf('authority-connector-profile_has_grant')                     >= 0) return { kind: 'authority_profile_grant',   label: 'Profile has grant' };
    if (s.indexOf('authority-connector-grant_authorises_agent')                >= 0) return { kind: 'authority_grant_agent',     label: 'Grant authorises agent' };
    if (s.indexOf('authority-connector-surface_has_fail_mode_policy')          >= 0) return { kind: 'authority_surface_fmp',     label: 'Surface fail-mode override' };
    if (s.indexOf('authority-connector-business_service_has_fail_mode_policy') >= 0) return { kind: 'authority_bs_fmp',          label: 'Business service fail-mode default' };
    if (s.indexOf('authority-connector-profile_escalates_to')                  >= 0) return { kind: 'authority_profile_escalation', label: 'Profile escalates to' };
    // Generic Authority connector marker (no edge-kind suffix yet — e.g.
    // future tranches' synthetic Authority edges) gets a label too.
    if (s.indexOf('authority-connector')                                       >= 0) return { kind: 'authority',                 label: 'Authority connector' };
    return { kind: 'unknown', label: 'Connector' };
  }

  window.MIDASGovernanceMap.gmapNodeCategoryFromKind = gmapNodeCategoryFromKind;
  window.MIDASGovernanceMap.gmapConnectorKindFromCls = gmapConnectorKindFromCls;
})();
