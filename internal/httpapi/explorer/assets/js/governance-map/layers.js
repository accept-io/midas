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
      case 'business':
      case 'related':   return 'business';
      case 'cap':       return 'capability';
      case 'proc':      return 'process';
      case 'surface':   return 'surface';
      case 'ai':        return 'ai';
      case 'authority':
      case 'coverage':  return 'synthetic';
      default:          return '';
    }
  }

  function gmapConnectorKindFromCls(cls) {
    const s = String(cls || '');
    if (s.indexOf('connector-ai-binding') >= 0) return { kind: 'ai_binding',   label: 'AI binding' };
    if (s.indexOf('connector-authority')  >= 0) return { kind: 'authority',    label: 'Authority' };
    if (s.indexOf('connector-evidence')   >= 0) return { kind: 'evidence',     label: 'Evidence' };
    if (s.indexOf('connector-gap')        >= 0) return { kind: 'coverage_gap', label: 'Coverage gap' };
    if (s.indexOf('connector-service')    >= 0) return { kind: 'service',      label: 'Service relationship' };
    return { kind: 'unknown', label: 'Connector' };
  }

  window.MIDASGovernanceMap.gmapNodeCategoryFromKind = gmapNodeCategoryFromKind;
  window.MIDASGovernanceMap.gmapConnectorKindFromCls = gmapConnectorKindFromCls;
})();
