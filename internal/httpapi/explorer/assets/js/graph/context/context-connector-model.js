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

  // ── Public API ─────────────────────────────────────────────────────

  // buildConnectorForEdge — produce a single ContextConnector for the
  // given projection edge.
  function buildConnectorForEdge(edge, projection) {
    if (!_isPlainObject(edge)) return null;
    if (EDGE_KINDS.indexOf(_str(edge.kind)) < 0) return null;
    var tpl = _templateForEdge(edge, projection);
    if (!tpl) return null;

    var srcKind = _str(edge.src && edge.src.kind);
    var srcId   = _str(edge.src && edge.src.id);
    var dstKind = _str(edge.dst && edge.dst.kind);
    var dstId   = _str(edge.dst && edge.dst.id);

    return {
      id:             _edgeId(edge),
      edgeKind:       _str(edge.kind),
      source:         { kind: srcKind, id: srcId },
      target:         { kind: dstKind, id: dstId },
      semanticClass:  tpl.semantic,
      visualClass:    tpl.visual,
      directionality: tpl.dir,
      strokeFamily:   tpl.stroke,
      dashPattern:    tpl.dash,
      priority:       PRIORITY[tpl.visual] || 1,
      label:          null,
      hoverLabel:     null,
      ariaText:       _ariaTextForEdge(edge),
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
    VISUAL_CLASSES:                VISUAL_CLASSES,
    SEMANTIC_CLASSES:              SEMANTIC_CLASSES,
    STROKE_FAMILIES:               STROKE_FAMILIES,
    PRIORITY:                      PRIORITY,
    buildConnectorsFromProjection: buildConnectorsFromProjection,
    buildConnectorForEdge:         buildConnectorForEdge,
    connectorVisualClassForEdge:   connectorVisualClassForEdge,
  };
})();
