// /explorer/assets/js/graph/context/context-connector-model.js
//
// D37o-impl-1 — Context Connector Model.
//
// Pure-data, renderer-independent builder that converts Context Graph
// projection edges into an array of `ContextConnector` specs. The
// model is the input contract for any future Context renderer's edge
// painter; it carries no dependency on path rendering or graph
// engines.
//
// Source contract: the projection envelope shape locked in
// internal/graph/context/projection.go (Edge { kind, src, dst, label };
// Projection { root, view, depth, nodes, edges }).
//
// Output contract: see D37o-design-1 §5 (ContextConnector shape).
//
// Public surface (attached to MIDASExplorerGraph.contextModels.connector):
//   EDGE_KINDS                     — frozen list of the 8 Context edge kinds
//   VISUAL_CLASSES                 — frozen list of the 5 visual classes
//   SEMANTIC_CLASSES               — frozen list of semantic classes
//   STROKE_FAMILIES                — frozen list of stroke families
//   buildConnectorsFromProjection  — projection → ContextConnector[]
//   buildConnectorForEdge          — (edge, projection) → ContextConnector
//   connectorVisualClassForEdge    — (edge, projection) → visual class string
//
// Working decision (D37o-design-1 §5.3, D-CONN-1):
//   reports_coverage defaults to the `authority` visual class. It is
//   promoted to the `gap` visual class when the source Coverage node
//   indicates a coverage gap. The locked source field is
//   `surfaces_with_no_ai_binding` on the Coverage typed-data block
//   (internal/graph/context/projection.go:367-372): a strictly
//   positive value indicates uncovered surfaces and promotes the
//   connector to the `gap` class. The implementation pins this
//   predicate in test pins; if a future tranche identifies a richer
//   gap signal, the predicate may be refined additively.
//
// Constraints (D37o-design-1 §5.1):
//   - No SVG path generation.
//   - No graph-engine edge objects.
//   - No DOM mutation.
//   - No legacy CSS class concatenation.
//   - No dependency on the dormant overlay-spike module.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph              = window.MIDASExplorerGraph || {};
  window.MIDASExplorerGraph.contextModels = window.MIDASExplorerGraph.contextModels || {};

  // ── Constants ──────────────────────────────────────────────────────

  var EDGE_KINDS = Object.freeze([
    'relates_to',
    'has_capability',
    'has_process',
    'has_surface',
    'bound_to',
    'system_of',
    'summarises',
    'reports_coverage',
  ]);

  // Five visual connector classes. `evidence` is reserved for a future
  // runtime-evidence edge kind not currently present in the backend
  // projection; the taxonomy carries it for forward compatibility.
  var VISUAL_CLASSES = Object.freeze([
    'service',
    'ai_binding',
    'authority',
    'evidence',
    'gap',
  ]);

  var SEMANTIC_CLASSES = Object.freeze([
    'structural',
    'functional',
    'synthesis',
    'evidence',
    'risk',
  ]);

  var STROKE_FAMILIES = Object.freeze([
    'neutral',
    'ai',
    'authority',
    'surface',
    'risk',
  ]);

  var CONNECTOR_FAMILIES = Object.freeze([
    'structural',
    'dependency',
    'runtime_operational',
    'evidence',
    'drift_risk_exception',
    'semantic_contextual',
    'authority_governance',
  ]);

  var CONNECTOR_TYPES = Object.freeze([
    'service_contains_capability',
    'service_supports_process',
    'process_uses_surface',
    'service_depends_on_service',
    'ai_binding_applies_to_scope',
    'ai_binding_uses_system',
    'authority_summary_informs_service',
    'coverage_reports_on_service',
    'coverage_reports_gap_for_service',
  ]);

  var DIRECTION_SOURCE_TO_TARGET = 'source_to_target';
  var DIRECTION_ASSOCIATIVE      = 'associative';
  var ARROW_DIRECTED             = 'directed';
  var ARROW_NONE                 = 'none';
  var LABEL_POLICY_HOVER         = 'hover';
  var WEIGHT_POLICY_NONE         = 'none';

  // Dash patterns are emitted as { on, off } in pixels. Solid renders
  // as the literal string 'solid' so consumers can branch cheaply
  // without inspecting object shape.
  var DASH_SOLID  = 'solid';
  var DASH_6_4    = Object.freeze({ on: 6, off: 4 });
  var DASH_5_5    = Object.freeze({ on: 5, off: 5 });

  // Priority is a z-order hint; higher wins when connectors overlap.
  // gap (4) > authority (3) > ai_binding (2) > service / evidence (1).
  var PRIORITY = Object.freeze({
    service:    1,
    evidence:   1,
    ai_binding: 2,
    authority:  3,
    gap:        4,
  });

  // Base mapping: edge kind → connector spec template. Per D37o-design-1
  // §5.3. `reports_coverage` is intentionally absent here; its visual
  // class is resolved by the predicate `_resolveReportsCoverage`.
  var BASE_MAPPING = Object.freeze({
    has_capability:   { semantic: 'structural', visual: 'service',    stroke: 'neutral',   dash: DASH_SOLID, dir: 'directed' },
    has_process:      { semantic: 'structural', visual: 'service',    stroke: 'neutral',   dash: DASH_SOLID, dir: 'directed' },
    has_surface:      { semantic: 'structural', visual: 'service',    stroke: 'neutral',   dash: DASH_SOLID, dir: 'directed' },
    relates_to:       { semantic: 'structural', visual: 'service',    stroke: 'neutral',   dash: DASH_SOLID, dir: 'undirected' },
    bound_to:         { semantic: 'functional', visual: 'ai_binding', stroke: 'ai',        dash: DASH_SOLID, dir: 'directed' },
    system_of:        { semantic: 'functional', visual: 'ai_binding', stroke: 'ai',        dash: DASH_SOLID, dir: 'directed' },
    summarises:       { semantic: 'synthesis',  visual: 'authority',  stroke: 'authority', dash: DASH_6_4,   dir: 'directed' },
  });

  var TAXONOMY_BY_EDGE_KIND = Object.freeze({
    has_capability: {
      connectorType: 'service_contains_capability',
      family: 'structural',
      label: 'contains',
      hoverTemplate: 'Business Service contains Capability',
    },
    has_process: {
      connectorType: 'service_supports_process',
      family: 'dependency',
      label: 'supports',
      hoverTemplate: 'Business Service supports Process',
    },
    has_surface: {
      connectorType: 'process_uses_surface',
      family: 'dependency',
      label: 'uses surface',
      hoverTemplate: 'Process uses Decision Surface',
    },
    relates_to: {
      connectorType: 'service_depends_on_service',
      family: 'dependency',
      label: 'depends on',
      hoverTemplate: 'Business Service depends on Business Service',
    },
    bound_to: {
      connectorType: 'ai_binding_applies_to_scope',
      family: 'dependency',
      label: 'applies to',
      hoverTemplate: 'AI Binding applies to selected scope',
    },
    system_of: {
      connectorType: 'ai_binding_uses_system',
      family: 'dependency',
      label: 'uses system',
      hoverTemplate: 'AI Binding uses AI System',
    },
    summarises: {
      connectorType: 'authority_summary_informs_service',
      family: 'authority_governance',
      label: 'informs',
      hoverTemplate: 'Authority Summary informs Business Service',
    },
  });

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _num(v) {
    if (typeof v === 'number' && isFinite(v)) return v;
    return 0;
  }

  function _refStr(ref) {
    if (!_isPlainObject(ref)) return '';
    return _str(ref.kind) + ':' + _str(ref.id);
  }

  function _edgeId(edge) {
    if (!_isPlainObject(edge)) return '';
    return _str(edge.kind) + '|' + _refStr(edge.src) + '|' + _refStr(edge.dst);
  }

  function _findNode(projection, ref) {
    if (!_isPlainObject(projection) || !Array.isArray(projection.nodes)) return null;
    if (!_isPlainObject(ref)) return null;
    var k = _str(ref.kind);
    var id = _str(ref.id);
    for (var i = 0; i < projection.nodes.length; i++) {
      var n = projection.nodes[i];
      if (_str(n.kind) === k && _str(n.id) === id) return n;
    }
    return null;
  }

  // _resolveReportsCoverage implements the D-CONN-1 working decision.
  // The source field is locked: a strictly positive
  // `surfaces_with_no_ai_binding` on the Coverage typed-data block
  // indicates uncovered surfaces and promotes the connector to `gap`.
  // If the source Coverage node is not in the projection, the
  // connector defaults to `authority` (synthesis).
  function _resolveReportsCoverage(edge, projection) {
    var coverageNode = _findNode(projection, edge.src);
    var coverageData = (coverageNode && _isPlainObject(coverageNode.coverage)) ? coverageNode.coverage : null;
    var hasGap = false;
    if (coverageData) {
      hasGap = _num(coverageData.surfaces_with_no_ai_binding) > 0;
    }
    if (hasGap) {
      return { semantic: 'risk',      visual: 'gap',       stroke: 'risk',      dash: DASH_5_5, dir: 'directed' };
    }
    return { semantic: 'synthesis', visual: 'authority', stroke: 'authority', dash: DASH_6_4, dir: 'directed' };
  }

  function _templateForEdge(edge, projection) {
    var k = _str(edge.kind);
    if (k === 'reports_coverage') return _resolveReportsCoverage(edge, projection);
    return BASE_MAPPING[k] || null;
  }

  function _ariaTextForEdge(edge) {
    var k = _str(edge.kind).replace(/_/g, ' ');
    return _str(edge.src && edge.src.kind) + ' ' + k + ' ' + _str(edge.dst && edge.dst.kind);
  }

  function _kindLabel(kind) {
    var s = _str(kind).replace(/[_-]+/g, ' ');
    return s.replace(/\b\w/g, function (m) { return m.toUpperCase(); });
  }

  function _taxonomyForEdge(edge, tpl) {
    var kind = _str(edge && edge.kind);
    if (kind === 'reports_coverage') {
      var isGap = tpl && tpl.visual === 'gap';
      return {
        connectorType: isGap ? 'coverage_reports_gap_for_service' : 'coverage_reports_on_service',
        family: isGap ? 'drift_risk_exception' : 'evidence',
        label: isGap ? 'coverage gap' : 'reports coverage',
        hoverTemplate: isGap ? 'Coverage reports gap for Business Service' : 'Coverage reports on Business Service',
      };
    }
    return TAXONOMY_BY_EDGE_KIND[kind] || {
      connectorType: kind || 'unknown_context_connector',
      family: 'semantic_contextual',
      label: kind ? kind.replace(/_/g, ' ') : 'relates to',
      hoverTemplate: 'Context relationship',
      fallback: true,
    };
  }

  function _directionForTemplate(tpl) {
    return tpl && tpl.dir === 'undirected' ? DIRECTION_ASSOCIATIVE : DIRECTION_SOURCE_TO_TARGET;
  }

  function _arrowPolicyForTemplate(tpl) {
    return tpl && tpl.dir === 'undirected' ? ARROW_NONE : ARROW_DIRECTED;
  }

  function _accessibilityLabel(edge, taxonomy) {
    var src = _kindLabel(edge && edge.src && edge.src.kind);
    var dst = _kindLabel(edge && edge.dst && edge.dst.kind);
    var label = taxonomy && taxonomy.label ? taxonomy.label : 'relates to';
    return (src + ' ' + label + ' ' + dst).replace(/\s+/g, ' ').trim();
  }

  function _hoverSummary(edge, taxonomy) {
    if (taxonomy && taxonomy.hoverTemplate) return taxonomy.hoverTemplate;
    return _accessibilityLabel(edge, taxonomy);
  }

  // ── Public API ─────────────────────────────────────────────────────

  // buildConnectorForEdge — produce a single ContextConnector for the
  // given projection edge.
  function buildConnectorForEdge(edge, projection) {
    if (!_isPlainObject(edge)) return null;
    if (EDGE_KINDS.indexOf(_str(edge.kind)) < 0) return null;
    var tpl = _templateForEdge(edge, projection);
    if (!tpl) return null;
    var taxonomy = _taxonomyForEdge(edge, tpl);

    var srcKind = _str(edge.src && edge.src.kind);
    var srcId   = _str(edge.src && edge.src.id);
    var dstKind = _str(edge.dst && edge.dst.kind);
    var dstId   = _str(edge.dst && edge.dst.id);

    return {
      id:             _edgeId(edge),
      edgeKind:       _str(edge.kind),
      connectorType:  taxonomy.connectorType,
      family:         taxonomy.family,
      source:         { kind: srcKind, id: srcId },
      target:         { kind: dstKind, id: dstId },
      semanticClass:  tpl.semantic,
      visualClass:    tpl.visual,
      directionality: tpl.dir,
      direction:      _directionForTemplate(tpl),
      arrowPolicy:    _arrowPolicyForTemplate(tpl),
      labelPolicy:    LABEL_POLICY_HOVER,
      weightPolicy:   WEIGHT_POLICY_NONE,
      statePolicy:    taxonomy.family === 'drift_risk_exception' ? 'risk_state' : 'active',
      strokeFamily:   tpl.stroke,
      dashPattern:    tpl.dash,
      priority:       PRIORITY[tpl.visual] || 1,
      label:          taxonomy.label,
      hoverLabel:     taxonomy.label,
      hoverSummary:   _hoverSummary(edge, taxonomy),
      accessibilityLabel: _accessibilityLabel(edge, taxonomy),
      fallback:       taxonomy.fallback === true,
      ariaText:       _accessibilityLabel(edge, taxonomy) || _ariaTextForEdge(edge),
      sourceEdgeRef:  {
        kind: _str(edge.kind),
        src:  { kind: srcKind, id: srcId },
        dst:  { kind: dstKind, id: dstId },
      },
    };
  }

  function buildConnectorsFromProjection(projection) {
    if (!_isPlainObject(projection)) return [];
    if (!Array.isArray(projection.edges)) return [];
    var out = [];
    for (var i = 0; i < projection.edges.length; i++) {
      var c = buildConnectorForEdge(projection.edges[i], projection);
      if (c) out.push(c);
    }
    return out;
  }

  // connectorVisualClassForEdge — convenience accessor that returns
  // only the visual class string. Used by callers that need to know
  // the class without constructing the full connector spec.
  function connectorVisualClassForEdge(edge, projection) {
    var c = buildConnectorForEdge(edge, projection);
    return c ? c.visualClass : '';
  }

  window.MIDASExplorerGraph.contextModels.connector = {
    EDGE_KINDS:                    EDGE_KINDS,
    CONNECTOR_TYPES:               CONNECTOR_TYPES,
    CONNECTOR_FAMILIES:            CONNECTOR_FAMILIES,
    VISUAL_CLASSES:                VISUAL_CLASSES,
    SEMANTIC_CLASSES:              SEMANTIC_CLASSES,
    STROKE_FAMILIES:               STROKE_FAMILIES,
    PRIORITY:                      PRIORITY,
    buildConnectorsFromProjection: buildConnectorsFromProjection,
    buildConnectorForEdge:         buildConnectorForEdge,
    connectorVisualClassForEdge:   connectorVisualClassForEdge,
  };
})();
