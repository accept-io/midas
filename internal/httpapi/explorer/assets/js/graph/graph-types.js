// /explorer/assets/js/graph/graph-types.js — D32a-impl-1
//
// Shared JSDoc typedefs and string-constant tables for the lens-
// agnostic Graph UI. Plain script; attaches to
// window.MIDASExplorerGraph.types. No runtime behaviour beyond
// exposing the constant tables.
//
// The Graph UI is being built lens-by-lens: Context first (this
// tranche packages the existing renderer behind the shell), then
// Authority (subsequent tranche). Both lenses share the normalised
// internal shape declared here:
//
//   /** @typedef {Object} GraphNode
//    *  @property {string} id
//    *  @property {string} kind   one of NODE_KIND_*
//    *  @property {string} [name]
//    *  @property {string} [status]      lens-specific posture
//    *  @property {object} [evidence]    lens-specific evidence map
//    *  @property {object} [meta]        free-form passthrough
//    */
//
//   /** @typedef {Object} GraphEdge
//    *  @property {string} from           node id
//    *  @property {string} to             node id
//    *  @property {string} kind           one of EDGE_KIND_*
//    *  @property {object} [meta]
//    */
//
//   /** @typedef {Object} GraphPayload
//    *  @property {GraphNode[]} nodes
//    *  @property {GraphEdge[]} edges
//    *  @property {object}      [summary]
//    *  @property {object[]}    [diagnostics]
//    *  @property {string}      lens       'context' | 'authority'
//    */
//
// Authority node kinds (D31l backend) — listed here for client-side
// type narrowing without requiring an OpenAPI lookup. Context lens
// uses its own kinds (service, capability, ai_system, …) and a
// future migration will normalise both into the same union; for
// today, lens adapters keep their kinds local.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // Authority lens (D31f → D31m backend; D32b-impl-1 front-end).
  // The constants below are the canonical client-side allow-list and
  // mirror the wire-level strings emitted by the backend Authority
  // Graph projection. Adding or removing a kind here without a backend
  // change is an error.
  var AUTHORITY_NODE_KINDS = Object.freeze([
    'business_service',
    'decision_surface',
    'authority_profile',
    'authority_grant',
    'agent',
    'fail_mode_policy',
    'escalation_target',
  ]);
  var AUTHORITY_EDGE_KINDS = Object.freeze([
    'business_service_has_surface',
    'surface_uses_profile',
    'profile_has_grant',
    'grant_authorises_agent',
    'surface_has_fail_mode_policy',
    'business_service_has_fail_mode_policy',
    'profile_escalates_to',
  ]);

  // Context lens (legacy /v1/graphs/context payload).
  var CONTEXT_NODE_KINDS = Object.freeze([
    'business_service',
    'capability',
    'process',
    'decision_surface',
    'profile',
    'ai_system',
    'agent',
  ]);

  // Lens identifiers as used by the store / router / lens switcher.
  var LENSES = Object.freeze({
    CONTEXT:   'context',
    AUTHORITY: 'authority',
  });

  window.MIDASExplorerGraph.types = {
    AUTHORITY_NODE_KINDS: AUTHORITY_NODE_KINDS,
    AUTHORITY_EDGE_KINDS: AUTHORITY_EDGE_KINDS,
    CONTEXT_NODE_KINDS:   CONTEXT_NODE_KINDS,
    LENSES:               LENSES,
  };
})();
