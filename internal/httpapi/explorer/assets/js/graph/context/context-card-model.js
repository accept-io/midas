// /explorer/assets/js/graph/context/context-card-model.js
//
// D37o-impl-1 — Context Card Model.
//
// Pure-data, renderer-independent builder that converts a Context Graph
// projection envelope into an array of `ContextCard` specs. The model
// is the input contract for any future Context renderer; it carries no
// dependency on rendering targets, graph engines, or selection
// surfaces.
//
// Source contract: the projection envelope shape locked in
// internal/graph/context/projection.go (Node + per-kind typed-data
// blocks; Edge; Projection { root, view, depth, nodes, edges }).
//
// Output contract: see D37o-design-1 §4 (ContextCard shape).
//
// Public surface (attached to MIDASExplorerGraph.contextModels.card):
//   NODE_KINDS                 — frozen list of the 9 Context node kinds
//   BADGE_CLASSES              — frozen list of the badge class vocab
//   ACTION_KINDS               — frozen list of the action descriptor kinds
//   ROLES                      — frozen list of card role values
//   buildCardsFromProjection   — projection → ContextCard[]
//   buildCardForNode           — (node, projection, options) → ContextCard
//
// Constraints (D37o-design-1 §4.1):
//   - No DOM, no rendering, no selection-side-effect calls.
//   - No reference to right-drawer setters or tray APIs.
//   - No reference to legacy renderer-owned identifiers.
//   - No dependency on the dormant overlay-spike module.
//   - Renderer-purity is enforced by a meta-test.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph              = window.MIDASExplorerGraph || {};
  window.MIDASExplorerGraph.contextModels = window.MIDASExplorerGraph.contextModels || {};

  // ── Constants ──────────────────────────────────────────────────────

  var NODE_KINDS = Object.freeze([
    'business_service',
    'related_business_service',
    'capability',
    'process',
    'decision_surface',
    'ai_system',
    'ai_system_binding',
    'authority_summary',
    'coverage',
  ]);

  // Frozen badge-class vocabulary. Renderers map these to visual styles;
  // the model emits only the symbolic class name and the human label.
  var BADGE_CLASSES = Object.freeze([
    'fmp-default',
    'fmp-inherited',
    'fmp-override',
    'ai-bind',
    'ai-warn',
    'coverage-ok',
    'coverage-warn',
  ]);

  var ACTION_KINDS = Object.freeze([
    'reframe-around-this',  // payload: targetView + targetId + label
    'view-business-service-record',
    'view-capability-record',
  ]);
  // Action payload vocabulary (D37o-design-1 §4.2): every descriptor
  // emitted by this model carries the camelCase fields { targetId,
  // targetView, label, surface }. The selection bridge translates to
  // the legacy snake_case wire shape { target_id, target_view, label }
  // expected by `_actionDispatcher` / `handleGovernanceMapAction`.

  var ROLES = Object.freeze(['root', 'governance', 'layer']);

  // Eyebrow labels (uppercase variant) mirror the kind-label vocabulary
  // owned by context-graph-adapter.js but are owned here for the model.
  // The renderer uses these verbatim.
  var EYEBROW_BY_KIND = Object.freeze({
    business_service:         'BUSINESS SERVICE',
    related_business_service: 'RELATED SERVICE',
    capability:               'CAPABILITY',
    process:                  'PROCESS',
    decision_surface:         'DECISION SURFACE',
    ai_system:                'AI SYSTEM',
    ai_system_binding:        'AI BINDING',
    authority_summary:        'AUTHORITY SUMMARY',
    coverage:                 'COVERAGE',
  });

  var DISPLAY_LABEL_BY_KIND = Object.freeze({
    business_service:         'Business Service',
    related_business_service: 'Related Business Service',
    capability:               'Capability',
    process:                  'Process',
    decision_surface:         'Decision Surface',
    ai_system:                'AI System',
    ai_system_binding:        'AI Binding',
    authority_summary:        'Authority Summary',
    coverage:                 'Coverage',
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

  function _isNodeRefEqual(a, b) {
    if (!a || !b) return false;
    return _str(a.kind) === _str(b.kind) && _str(a.id) === _str(b.id);
  }

  function _typedDataForNode(node) {
    if (!_isPlainObject(node)) return null;
    var k = _str(node.kind);
    if (!k) return null;
    var block = node[k];
    return _isPlainObject(block) ? block : null;
  }

  function _emptyMetaRow(text, emphasis) {
    return { text: _str(text), emphasis: emphasis || 'none' };
  }

  function _newBadge(cls, text, tooltip) {
    var b = { cls: _str(cls), text: _str(text) };
    if (tooltip) b.tooltip = _str(tooltip);
    return b;
  }

  function _newMetric(key, label, value) {
    var v = value;
    if (typeof v !== 'number' && typeof v !== 'string') v = _str(v);
    return { key: _str(key), label: _str(label), value: v };
  }

  function _newAction(kind, opts) {
    var a = { kind: _str(kind) };
    if (opts) {
      if (opts.targetId)   a.targetId   = _str(opts.targetId);
      if (opts.targetView) a.targetView = _str(opts.targetView);
      if (opts.label)      a.label      = _str(opts.label);
      a.surface = opts.surface === 'detail' ? 'detail' : 'inline';
    } else {
      a.surface = 'inline';
    }
    return a;
  }

  function _baseState() {
    return { selected: false, hovered: false, dimmed: false, focused: false };
  }

  function _ariaLabel(eyebrow, name) {
    return _str(eyebrow) + ': ' + _str(name);
  }

  function _roleFor(node, projection) {
    if (!node) return 'layer';
    if (_str(node.kind) === 'authority_summary' || _str(node.kind) === 'coverage') {
      return 'governance';
    }
    var root = projection && projection.root;
    if (_isNodeRefEqual({ kind: node.kind, id: node.id }, root)) return 'root';
    return 'layer';
  }

  function _cardId(node) {
    return _str(node.kind) + ':' + _str(node.id);
  }

  // ── Per-kind builders ──────────────────────────────────────────────

  function _buildBusinessService(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    if (data.status) meta.push(_emptyMetaRow(_str(data.status)));
    if (data.owner)  meta.push(_emptyMetaRow('Owner ' + _str(data.owner)));

    var badges = [];
    if (data.fail_mode_policy_id) {
      badges.push(_newBadge('fmp-default', 'FMP default'));
    }

    var details = {};
    if (data.status)              details.status              = _str(data.status);
    if (data.owner)               details.owner               = _str(data.owner);
    if (data.service_type)        details.service_type        = _str(data.service_type);
    if (data.regulatory_scope)    details.regulatory_scope    = _str(data.regulatory_scope);
    if (data.fail_mode_policy_id) details.fail_mode_policy_id = _str(data.fail_mode_policy_id);
    if (data.description)         details.description         = _str(data.description);
    if (data.external_ref && data.external_ref.source_system) {
      details.external_ref_source = _str(data.external_ref.source_system);
    }

    var actions = [
      _newAction('view-business-service-record', {
        targetId: _str(node.id),
        label:    'Open business service record',
        surface:  'inline',
      }),
    ];

    return _composeCard(node, role, meta, badges, [], details, actions);
  }

  function _buildRelatedBusinessService(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    var relRow = data.outgoing || data.incoming;
    var relType = relRow && relRow.relationship_type;
    if (relType) meta.push(_emptyMetaRow(_str(relType)));
    if (relRow && relRow.description) {
      meta.push(_emptyMetaRow(_str(relRow.description)));
    }

    var details = {};
    if (data.id) details.target_id = _str(data.id);
    if (relType) details.relationship = _str(relType);
    if (relRow && relRow.relationship_id) {
      details.relationship_id = _str(relRow.relationship_id);
    }

    var actions = [
      _newAction('reframe-around-this', {
        targetId:   _str(node.id),
        targetView: 'service',
        label:      'Reframe around this service',
        surface:    'inline',
      }),
      _newAction('view-business-service-record', {
        targetId: _str(node.id),
        label:    'Open business service record',
        surface:  'detail',
      }),
    ];

    return _composeCard(node, role, meta, [], [], details, actions);
  }

  function _buildCapability(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    if (data.status) meta.push(_emptyMetaRow(_str(data.status)));

    var details = {};
    if (data.status)      details.status      = _str(data.status);
    if (data.owner)       details.owner       = _str(data.owner);
    if (data.description) details.description = _str(data.description);

    var actions = [
      _newAction('view-capability-record', {
        targetId: _str(node.id),
        label:    'Open capability record',
        surface:  'inline',
      }),
    ];

    return _composeCard(node, role, meta, [], [], details, actions, _str(data.description));
  }

  function _buildProcess(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    if (data.status) meta.push(_emptyMetaRow(_str(data.status)));

    var details = {};
    if (data.status)              details.status              = _str(data.status);
    if (data.owner)               details.owner               = _str(data.owner);
    if (data.business_service_id) details.business_service_id = _str(data.business_service_id);
    if (data.description)         details.description         = _str(data.description);

    return _composeCard(node, role, meta, [], [], details, []);
  }

  function _buildDecisionSurface(node, projection, role) {
    var data = _typedDataForNode(node) || {};

    var bindingIds = Array.isArray(data.ai_binding_ids) ? data.ai_binding_ids : [];
    var inheritedBindingIds = Array.isArray(data.inherited_ai_binding_ids) ? data.inherited_ai_binding_ids : [];
    var aiBindingCount = bindingIds.length + inheritedBindingIds.length;

    var meta = [];
    if (typeof data.version === 'number') meta.push(_emptyMetaRow('v' + data.version));
    meta.push(_emptyMetaRow(aiBindingCount + ' AI binding' + (aiBindingCount === 1 ? '' : 's')));

    // FMP badge: override when the surface carries its own policy id;
    // inherited when it does not (the renderer can use BS-level lookup
    // for the "Inherited default" copy; the model only flags the state).
    var badges = [];
    if (data.fail_mode_policy_id) {
      badges.push(_newBadge('fmp-override', 'FMP override'));
    } else {
      badges.push(_newBadge('fmp-inherited', 'FMP inherited'));
    }
    if (aiBindingCount > 0) {
      badges.push(_newBadge('ai-bind', 'AI bound'));
    }

    var metrics = [];
    metrics.push(_newMetric('profile_count', 'profiles', _num(data.profile_count)));
    metrics.push(_newMetric('grant_count',   'grants',   _num(data.grant_count)));
    metrics.push(_newMetric('agent_count',   'agents',   _num(data.agent_count)));

    var details = {};
    if (data.status)              details.status              = _str(data.status);
    if (data.process_id)          details.process_id          = _str(data.process_id);
    if (typeof data.version === 'number') details.version = data.version;
    if (data.fail_mode_policy_id) details.fail_mode_policy_id = _str(data.fail_mode_policy_id);
    if (aiBindingCount > 0) {
      details.ai_binding_count           = aiBindingCount;
      details.direct_ai_binding_count    = bindingIds.length;
      details.inherited_ai_binding_count = inheritedBindingIds.length;
    }
    details.profile_count = _num(data.profile_count);
    details.grant_count   = _num(data.grant_count);
    details.agent_count   = _num(data.agent_count);
    if (data.description) details.description = _str(data.description);

    var actions = [];
    if (role !== 'root') {
      actions.push(_newAction('reframe-around-this', {
        targetId:   _str(node.id),
        targetView: 'decision_surface',
        label:      'Reframe around this surface',
        surface:    'inline',
      }));
    }

    return _composeCard(node, role, meta, badges, metrics, details, actions);
  }

  function _buildAISystem(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    if (data.system_type) meta.push(_emptyMetaRow(_str(data.system_type)));
    if (data.active_version_label) meta.push(_emptyMetaRow('v' + _str(data.active_version_label)));

    var badges = [];
    if (data.active_version_status && _str(data.active_version_status).toLowerCase() !== 'active') {
      badges.push(_newBadge('ai-warn', _str(data.active_version_status)));
    }

    var details = {};
    if (data.vendor)              details.vendor               = _str(data.vendor);
    if (data.system_type)         details.system_type          = _str(data.system_type);
    if (data.status)              details.status               = _str(data.status);
    if (data.active_version != null) details.active_version    = data.active_version;
    if (data.active_version_label)   details.active_version_label  = _str(data.active_version_label);
    if (data.active_version_status)  details.active_version_status = _str(data.active_version_status);
    if (data.description)         details.description          = _str(data.description);
    if (data.external_ref && data.external_ref.source_system) {
      details.external_ref_source = _str(data.external_ref.source_system);
    }

    var actions = [];
    if (role !== 'root') {
      actions.push(_newAction('reframe-around-this', {
        targetId:   _str(node.id),
        targetView: 'ai_system',
        label:      'Reframe around this AI system',
        surface:    'inline',
      }));
    }

    var subtitle = data.vendor ? _str(data.vendor) : '';
    return _composeCard(node, role, meta, badges, [], details, actions, subtitle);
  }

  function _buildAISystemBinding(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var meta = [];
    if (data.scope_label) meta.push(_emptyMetaRow(_str(data.scope_label)));
    if (data.role)        meta.push(_emptyMetaRow(_str(data.role)));

    var details = {};
    if (data.ai_system_id)   details.ai_system_id   = _str(data.ai_system_id);
    if (data.ai_system_name) details.ai_system_name = _str(data.ai_system_name);
    if (data.scope_kind)     details.scope_kind     = _str(data.scope_kind);
    if (data.scope_id)       details.scope_id       = _str(data.scope_id);
    if (data.scope_label)    details.scope_label    = _str(data.scope_label);
    if (data.role)           details.role           = _str(data.role);
    if (data.description)    details.description    = _str(data.description);

    return _composeCard(node, role, meta, [], [], details, []);
  }

  function _buildAuthoritySummary(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var metrics = [
      _newMetric('surface_count',        'surfaces', _num(data.surface_count)),
      _newMetric('active_profile_count', 'profiles', _num(data.active_profile_count)),
      _newMetric('active_grant_count',   'grants',   _num(data.active_grant_count)),
      _newMetric('active_agent_count',   'agents',   _num(data.active_agent_count)),
    ];
    var details = {};
    details.surface_count        = _num(data.surface_count);
    details.active_profile_count = _num(data.active_profile_count);
    details.active_grant_count   = _num(data.active_grant_count);
    details.active_agent_count   = _num(data.active_agent_count);

    return _composeCard(node, 'governance', [], [], metrics, details, []);
  }

  function _buildCoverage(node, projection, role) {
    var data = _typedDataForNode(node) || {};
    var metrics = [
      _newMetric('surface_count',                   'surfaces',     _num(data.surface_count)),
      _newMetric('surfaces_with_direct_ai_binding', 'direct',       _num(data.surfaces_with_direct_ai_binding)),
      _newMetric('surfaces_with_scoped_ai_binding', 'scoped',       _num(data.surfaces_with_scoped_ai_binding)),
      _newMetric('surfaces_with_no_ai_binding',     'no binding',   _num(data.surfaces_with_no_ai_binding)),
    ];
    // Gap signal: the canonical Coverage gap predicate (locked by
    // D37o-design-1 §5.3 working decision). The connector model
    // consults the same predicate to promote the reports_coverage
    // connector to the `gap` visual class.
    var hasGap = _num(data.surfaces_with_no_ai_binding) > 0;
    var badges = [
      _newBadge(hasGap ? 'coverage-warn' : 'coverage-ok',
                hasGap ? 'Coverage gap' : 'Coverage ok'),
    ];
    var details = {};
    details.surface_count                   = _num(data.surface_count);
    details.surfaces_with_direct_ai_binding = _num(data.surfaces_with_direct_ai_binding);
    details.surfaces_with_scoped_ai_binding = _num(data.surfaces_with_scoped_ai_binding);
    details.surfaces_with_no_ai_binding     = _num(data.surfaces_with_no_ai_binding);
    details.has_gap                         = hasGap;

    return _composeCard(node, 'governance', [], badges, metrics, details, []);
  }

  function _composeCard(node, role, meta, badges, metrics, details, actions, subtitle) {
    var kind     = _str(node.kind);
    var eyebrow  = EYEBROW_BY_KIND[kind] || kind.toUpperCase();
    var name     = _str(node.label || (node.id || ''));
    var card = {
      id:            _cardId(node),
      kind:          kind,
      role:          _str(role),
      label:         eyebrow,
      name:          name,
      meta:          Array.isArray(meta)    ? meta    : [],
      badges:        Array.isArray(badges)  ? badges  : [],
      metrics:       Array.isArray(metrics) ? metrics : [],
      state:         _baseState(),
      details:       _isPlainObject(details) ? details : {},
      actions:       Array.isArray(actions) ? actions : [],
      ariaLabel:     _ariaLabel(eyebrow, name),
      sourceNodeRef: { kind: kind, id: _str(node.id) },
    };
    if (subtitle) card.subtitle = _str(subtitle);
    return card;
  }

  // ── Public API ─────────────────────────────────────────────────────

  // buildCardForNode — produce a single ContextCard for the given
  // projection node. `options.rootNodeRef` overrides the projection's
  // root for the role assignment (useful for tests).
  function buildCardForNode(node, projection, options) {
    if (!_isPlainObject(node)) return null;
    if (NODE_KINDS.indexOf(_str(node.kind)) < 0) return null;

    var pseudoProjection = projection;
    if (options && options.rootNodeRef) {
      pseudoProjection = { root: options.rootNodeRef };
    }
    var role = _roleFor(node, pseudoProjection);

    switch (_str(node.kind)) {
      case 'business_service':         return _buildBusinessService(node, pseudoProjection, role);
      case 'related_business_service': return _buildRelatedBusinessService(node, pseudoProjection, role);
      case 'capability':               return _buildCapability(node, pseudoProjection, role);
      case 'process':                  return _buildProcess(node, pseudoProjection, role);
      case 'decision_surface':         return _buildDecisionSurface(node, pseudoProjection, role);
      case 'ai_system':                return _buildAISystem(node, pseudoProjection, role);
      case 'ai_system_binding':        return _buildAISystemBinding(node, pseudoProjection, role);
      case 'authority_summary':        return _buildAuthoritySummary(node, pseudoProjection, role);
      case 'coverage':                 return _buildCoverage(node, pseudoProjection, role);
      default:                         return null;
    }
  }

  // buildCardsFromProjection — produce ContextCard[] for every node in
  // the projection envelope. Nodes whose kind is outside the locked
  // vocabulary are skipped (returns a stable shape; never throws on
  // unknown kinds).
  function buildCardsFromProjection(projection) {
    if (!_isPlainObject(projection)) return [];
    if (!Array.isArray(projection.nodes)) return [];
    var out = [];
    for (var i = 0; i < projection.nodes.length; i++) {
      var c = buildCardForNode(projection.nodes[i], projection, null);
      if (c) out.push(c);
    }
    return out;
  }

  window.MIDASExplorerGraph.contextModels.card = {
    NODE_KINDS:               NODE_KINDS,
    BADGE_CLASSES:            BADGE_CLASSES,
    ACTION_KINDS:             ACTION_KINDS,
    ROLES:                    ROLES,
    EYEBROW_BY_KIND:          EYEBROW_BY_KIND,
    DISPLAY_LABEL_BY_KIND:    DISPLAY_LABEL_BY_KIND,
    buildCardsFromProjection: buildCardsFromProjection,
    buildCardForNode:         buildCardForNode,
  };
})();
