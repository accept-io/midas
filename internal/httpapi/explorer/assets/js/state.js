// /explorer/assets/js/state.js — D27j-ui-foundation-4
//
// Establishes the window.MIDASExplorerState namespace and hoists three
// pure cache containers onto it. These are object maps that the inline
// IIFE mutates by property access, not by reassignment, so the local
// const bindings inside the IIFE point at the same object identity as
// the namespace properties — every existing reader/writer continues
// to mutate the same underlying object.
//
// Caches extracted in this tranche:
//
//   serviceRecordCache       — Business Service record-page payloads,
//                              keyed by service id. Filled by the
//                              services-record loader; read by the
//                              record renderer + sub-view transitions.
//
//   capabilityRecordCache    — Capability record-page payloads,
//                              keyed by capability id. Same shape.
//
//   explorerEnvelopeDetailsById — Full envelope payloads from the
//                              Explorer-isolated detail endpoint,
//                              keyed by envelope id. Filled by the
//                              records lazy-detail loader; read by the
//                              records detail rail and the Activity
//                              tab rendering.
//
// Loading/error sibling primitives (serviceRecordLoading,
// serviceRecordError, capabilityRecordLoading, capabilityRecordError,
// explorerEnvelopeDetailLoadingId, explorerEnvelopeDetailError) are
// frequently reassigned and remain inline for now per the
// D27j-ui-foundation-4 brief.

(function () {
  'use strict';

  window.MIDASExplorerState = window.MIDASExplorerState || {};

  // Use ||= idiom (defensive against multiple loads) so the SAME
  // object identity persists across script reloads. Existing inline
  // call-sites mutate by property access, so identity is what matters.
  window.MIDASExplorerState.serviceRecordCache =
    window.MIDASExplorerState.serviceRecordCache || {};
  window.MIDASExplorerState.capabilityRecordCache =
    window.MIDASExplorerState.capabilityRecordCache || {};
  window.MIDASExplorerState.explorerEnvelopeDetailsById =
    window.MIDASExplorerState.explorerEnvelopeDetailsById || {};
})();
