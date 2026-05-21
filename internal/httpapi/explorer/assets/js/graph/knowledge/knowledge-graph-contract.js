// /explorer/assets/js/graph/knowledge/knowledge-graph-contract.js
// D36b — Knowledge Graph frontend projection contract — constants.
//
// Purely declarative. No runtime, no DOM, no fetch, no Cytoscape.
// This module exists so the D36b contract document
// (docs/design/D36b-knowledge-graph-frontend-projection-contract.md)
// and the future Knowledge Graph renderer stay in sync on the
// renderer id, the canonical node-kind taxonomy, and the canonical
// edge-kind taxonomy.
//
// Loaded by index.html after graph-viewport.js and BEFORE the D36a
// Knowledge shell (knowledge-graph-renderer.js) so the shell can
// consume `KNOWLEDGE_GRAPH_RENDERER_ID` at IIFE init and the
// literal id is declared in exactly one place. The shell also
// falls back to the literal `'knowledge-graph'` if this module
// failed to load, so a perturbed load order does not break the
// shell.
//
// If you change the node or edge kind lists below, update
// §7 and §8 of the contract document in the same change. The
// D36b tests pin both sides.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Renderer id (D36a) ───────────────────────────────────────────────

  var KNOWLEDGE_GRAPH_RENDERER_ID = 'knowledge-graph';

  // ── Projection envelope (D36b §6) ────────────────────────────────────

  var KNOWLEDGE_PROJECTION_VIEW = 'knowledge';

  // ── Canonical node kinds (D36b §7) ───────────────────────────────────
  //
  // Frozen, sorted array so iteration order is stable across
  // browsers and so consumers cannot mutate the canonical list.
  var KNOWLEDGE_NODE_KINDS = Object.freeze([
    'ai_system',
    'business_service',
    'capability',
    'concept',
    'control',
    'decision_surface',
    'evidence',
    'obligation',
    'policy',
    'risk_theme',
  ]);

  // Deferred node kinds — recognised in the namespace, not yet
  // canonical. Keep here so the namespace stays coherent.
  var KNOWLEDGE_DEFERRED_NODE_KINDS = Object.freeze([
    'concept_cluster',
    'metric',
  ]);

  // ── Canonical edge kinds (D36b §8) ───────────────────────────────────

  var KNOWLEDGE_EDGE_KINDS = Object.freeze([
    'applies_to',
    'constrains',
    'depends_on',
    'derived_from',
    'evidences',
    'governs',
    'implements',
    'mitigates',
    'relates_to',
    'supports',
  ]);

  // Deferred edge kinds — recognised, not yet canonical.
  var KNOWLEDGE_DEFERRED_EDGE_KINDS = Object.freeze([
    'aggregates',
    'measures',
  ]);

  // ── Public surface ───────────────────────────────────────────────────

  window.MIDASExplorerGraph.knowledgeGraphContract = {
    KNOWLEDGE_GRAPH_RENDERER_ID:    KNOWLEDGE_GRAPH_RENDERER_ID,
    KNOWLEDGE_PROJECTION_VIEW:      KNOWLEDGE_PROJECTION_VIEW,
    KNOWLEDGE_NODE_KINDS:           KNOWLEDGE_NODE_KINDS,
    KNOWLEDGE_DEFERRED_NODE_KINDS:  KNOWLEDGE_DEFERRED_NODE_KINDS,
    KNOWLEDGE_EDGE_KINDS:           KNOWLEDGE_EDGE_KINDS,
    KNOWLEDGE_DEFERRED_EDGE_KINDS:  KNOWLEDGE_DEFERRED_EDGE_KINDS,
  };
})();
