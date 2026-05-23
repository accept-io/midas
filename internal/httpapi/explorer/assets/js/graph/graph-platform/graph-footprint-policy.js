// /explorer/assets/js/graph/graph-platform/graph-footprint-policy.js
// D37o-overlap-8 — Minimal strategic graph footprint policy contract.
//
// Pure data only. No DOM, no Cytoscape, no measurement, no timers.
// The first supported consumer is strategic Context raw Cytoscape mode.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var GRAPH_SURFACE_CONTEXT = 'context';
  var RENDERER_MODE_RAW_CY  = 'raw-cytoscape';
  var SIZING_MODE_RAW_FIXED = 'raw-cytoscape-node-body-fixed';
  var SOURCE_CONTEXT_ESTIMATE = 'declared-context-estimate-migrated';

  var RAW_CONTEXT_GAP_X = 32;
  var RAW_CONTEXT_GAP_Y = 72;
  var RAW_CONTEXT_TOLERANCE = 0;
  var RAW_CONTEXT_DEFAULT = Object.freeze({ width: 220, height: 104 });

  var RAW_CONTEXT_BY_KIND = Object.freeze({
    business_service:         Object.freeze({ width: 220, height: 132 }),
    related_business_service: Object.freeze({ width: 220, height: 76  }),
    capability:               Object.freeze({ width: 220, height: 84  }),
    process:                  Object.freeze({ width: 220, height: 84  }),
    decision_surface:         Object.freeze({ width: 220, height: 104 }),
    ai_system:                Object.freeze({ width: 220, height: 96  }),
    ai_system_binding:        Object.freeze({ width: 220, height: 84  }),
    coverage:                 Object.freeze({ width: 220, height: 96  }),
    authority_summary:        Object.freeze({ width: 220, height: 96  })
  });

  function _str(v) {
    return (v == null) ? '' : String(v);
  }

  function _contextRawPolicyId(kind, variant) {
    var suffix = variant ? ('.' + variant) : '';
    return 'context.raw-cytoscape.' + (kind || 'unknown') + suffix + '.declared-v1';
  }

  function _resolveContextRaw(input) {
    var kind = _str(input && input.cardKind);
    var variant = _str(input && input.cardVariant) || null;
    var dims = RAW_CONTEXT_BY_KIND[kind] || RAW_CONTEXT_DEFAULT;
    return {
      policyId: _contextRawPolicyId(kind, variant),
      graphSurfaceId: GRAPH_SURFACE_CONTEXT,
      rendererMode: RENDERER_MODE_RAW_CY,
      cardKind: kind,
      cardVariant: variant,
      sizingMode: SIZING_MODE_RAW_FIXED,
      reservedWidth: dims.width,
      reservedHeight: dims.height,
      gapX: RAW_CONTEXT_GAP_X,
      gapY: RAW_CONTEXT_GAP_Y,
      tolerance: RAW_CONTEXT_TOLERANCE,
      source: SOURCE_CONTEXT_ESTIMATE,
      rawCytoscapeCompatible: true,
      htmlOverlayCompatible: false
    };
  }

  function resolve(input) {
    input = input || {};
    var surface = _str(input.graphSurfaceId);
    var mode = _str(input.rendererMode);
    if (surface === GRAPH_SURFACE_CONTEXT && mode === RENDERER_MODE_RAW_CY) {
      return _resolveContextRaw(input);
    }
    return {
      policyId: 'unsupported',
      graphSurfaceId: surface,
      rendererMode: mode,
      cardKind: _str(input.cardKind),
      cardVariant: input.cardVariant == null ? null : _str(input.cardVariant),
      sizingMode: 'unsupported',
      reservedWidth: RAW_CONTEXT_DEFAULT.width,
      reservedHeight: RAW_CONTEXT_DEFAULT.height,
      gapX: RAW_CONTEXT_GAP_X,
      gapY: RAW_CONTEXT_GAP_Y,
      tolerance: RAW_CONTEXT_TOLERANCE,
      source: 'unsupported',
      rawCytoscapeCompatible: false,
      htmlOverlayCompatible: false
    };
  }

  window.MIDASExplorerGraph.footprintPolicy = {
    resolve: resolve,
    _constants: {
      GRAPH_SURFACE_CONTEXT: GRAPH_SURFACE_CONTEXT,
      RENDERER_MODE_RAW_CY: RENDERER_MODE_RAW_CY,
      SIZING_MODE_RAW_FIXED: SIZING_MODE_RAW_FIXED,
      SOURCE_CONTEXT_ESTIMATE: SOURCE_CONTEXT_ESTIMATE,
      RAW_CONTEXT_GAP_X: RAW_CONTEXT_GAP_X,
      RAW_CONTEXT_GAP_Y: RAW_CONTEXT_GAP_Y,
      RAW_CONTEXT_BY_KIND: RAW_CONTEXT_BY_KIND
    }
  };
})();
