// /explorer/assets/js/graph/context/context-html-card-painter.js
//
// D37o-impl-3 — Context HTML Card Painter.
//
// A renderer-local DOM painter that consumes a `ContextCard` produced
// by the Context card model (D37o-impl-1) and returns a renderer-owned
// HTML element. The painter is a pure transformation: card spec in,
// DOM out. It owns no lifecycle, no projection acquisition, no
// selection routing, no fetch path.
//
// Architectural intent:
//
//   • The painter is the ONLY place in the Context renderer stack
//     that knows about per-kind card body shape. The card model
//     (D37o-impl-1) owns the per-kind data contract; this painter
//     translates that contract to DOM.
//   • The painter is renderer-agnostic with respect to the host
//     graph engine: it produces a self-contained DOM subtree that
//     any future graph renderer — overlay-based, flat DOM,
//     server-side-rendered, or otherwise — can host.
//   • Card slot vocabulary is preserved verbatim from D37o-design-1
//     §4.2: label (eyebrow) / name / subtitle / meta rows / badges /
//     metrics / actions. Per-kind variation is expressed via a CSS
//     hook class `context-card--{kind}`; the painter does NOT branch
//     on kind beyond emitting that class.
//   • Action descriptors are rendered as display-only DOM in this
//     tranche. Selection / reframe wiring is a later tranche.
//
// Public surface (window.MIDASExplorerGraph.contextCardPainter):
//
//   renderCard(card, options)     → HTMLElement
//   renderCardBody(card, options) → HTMLElement (the inner <div>)
//   renderBadge(badge)            → HTMLElement
//   renderMetric(metric)          → HTMLElement
//   _constants:                   { CARD_CLASS, KIND_CLASS_PREFIX, ... }
//
// Forbidden dependencies (architectural):
//   • No projection fetch or model-building inside the painter.
//   • No graph-engine references.
//   • No drawer setters or evidence-tray hooks.
//   • No legacy renderer DOM ids.
//   • No reference to the dormant overlay-spike module.
//   • No own state / no global mutation beyond the public-surface
//     registration.
//
// Naming policy:
//   The painter exposes `contextCardPainter` — durable product-level
//   identifier. No rollout-mode words leak into this surface.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var CARD_CLASS         = 'context-card';
  var KIND_CLASS_PREFIX  = 'context-card--';
  var HEADER_CLASS       = 'context-card-header';
  var EYEBROW_CLASS      = 'context-card-eyebrow';
  var NAME_CLASS         = 'context-card-name';
  var SUBTITLE_CLASS     = 'context-card-subtitle';
  var META_CLASS         = 'context-card-meta';
  var META_ROW_CLASS     = 'context-card-meta-row';
  var BADGES_CLASS       = 'context-card-badges';
  var BADGE_CLASS        = 'context-card-badge';
  var BADGE_CLASS_PREFIX = 'context-card-badge--';
  var METRICS_CLASS      = 'context-card-metrics';
  var METRIC_CLASS       = 'context-card-metric';
  var METRIC_LABEL_CLASS = 'context-card-metric-label';
  var METRIC_VALUE_CLASS = 'context-card-metric-value';
  var ACTIONS_CLASS      = 'context-card-actions';
  var ACTION_CLASS       = 'context-card-action';
  var ROLE_CLASS_PREFIX  = 'context-card--role-';
  var NODE_ACTIONS_CLASS = 'has-node-actions';
  var NODE_ACTION_TRIGGER_ATTR = 'data-graph-node-action-trigger';

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _el(tag, className) {
    var el = document.createElement(tag);
    if (className) el.className = className;
    return el;
  }

  function _setText(el, text) {
    el.textContent = _str(text);
    return el;
  }

  function _on(el, type, handler) {
    el.addEventListener(type, handler);
    return el;
  }

  // ── Public API ─────────────────────────────────────────────────────

  function renderCard(card, options) {
    if (!_isPlainObject(card)) {
      var fallback = _el('article', CARD_CLASS + ' ' + CARD_CLASS + '--empty');
      fallback.setAttribute('aria-label', 'Empty card');
      return fallback;
    }
    var opts = options || {};
    var kindCls = _str(card.kind) ? (KIND_CLASS_PREFIX + _str(card.kind)) : '';
    var roleCls = _str(card.role) ? (ROLE_CLASS_PREFIX + _str(card.role)) : '';
    var classes = CARD_CLASS;
    if (kindCls) classes += ' ' + kindCls;
    if (roleCls) classes += ' ' + roleCls;

    var actionContext = _nodeActionContext(card, opts);
    var hasNodeActions = _hasNodeActions(actionContext);
    if (hasNodeActions) classes += ' ' + NODE_ACTIONS_CLASS;

    var el = _el('article', classes);
    if (card.id)              el.setAttribute('data-card-id', _str(card.id));
    if (card.kind)            el.setAttribute('data-kind', _str(card.kind));
    if (card.role)            el.setAttribute('data-role', _str(card.role));
    if (card.ariaLabel)       el.setAttribute('aria-label', _str(card.ariaLabel));
    if (card.state && card.state.selected) el.setAttribute('aria-current', 'true');

    // D37o-impl-5 — card is interactive (click + keyboard) when a
    // host wires selection on the renderer mount. The painter just
    // exposes the affordance; the renderer/bridge owns behaviour.
    el.setAttribute('role', 'button');
    el.setAttribute('tabindex', '0');

    el.appendChild(renderCardBody(card, opts));
    el.appendChild(_renderNodeActionTrigger(actionContext, hasNodeActions));
    return el;
  }

  function renderCardBody(card, options) {
    var body = _el('div', 'context-card-body');
    body.appendChild(_renderHeader(card));
    if (Array.isArray(card.meta) && card.meta.length > 0) {
      body.appendChild(_renderMeta(card.meta));
    }
    if (Array.isArray(card.badges) && card.badges.length > 0) {
      body.appendChild(_renderBadgeList(card.badges));
    }
    if (Array.isArray(card.metrics) && card.metrics.length > 0) {
      body.appendChild(_renderMetricList(card.metrics));
    }
    if (Array.isArray(card.actions) && card.actions.length > 0) {
      body.appendChild(_renderActionList(card.actions, options));
    }
    void options;
    return body;
  }

  function renderBadge(badge) {
    if (!_isPlainObject(badge)) return _el('span', BADGE_CLASS);
    var classes = BADGE_CLASS;
    if (badge.cls) classes += ' ' + BADGE_CLASS_PREFIX + _str(badge.cls);
    var el = _el('span', classes);
    if (badge.cls) el.setAttribute('data-badge-cls', _str(badge.cls));
    if (badge.tooltip) el.setAttribute('title', _str(badge.tooltip));
    _setText(el, badge.text || '');
    return el;
  }

  function renderMetric(metric) {
    if (!_isPlainObject(metric)) return _el('li', METRIC_CLASS);
    var li = _el('li', METRIC_CLASS);
    if (metric.key) li.setAttribute('data-metric-key', _str(metric.key));
    var label = _el('span', METRIC_LABEL_CLASS);
    _setText(label, metric.label || metric.key || '');
    var value = _el('span', METRIC_VALUE_CLASS);
    _setText(value, metric.value == null ? '' : metric.value);
    li.appendChild(value);
    li.appendChild(label);
    return li;
  }

  // ── Internals ──────────────────────────────────────────────────────

  function _renderHeader(card) {
    var header = _el('header', HEADER_CLASS);
    if (card.label) {
      var eyebrow = _el('p', EYEBROW_CLASS);
      _setText(eyebrow, card.label);
      header.appendChild(eyebrow);
    }
    if (card.name) {
      var name = _el('h3', NAME_CLASS);
      _setText(name, card.name);
      header.appendChild(name);
    }
    if (card.subtitle) {
      var subtitle = _el('p', SUBTITLE_CLASS);
      _setText(subtitle, card.subtitle);
      header.appendChild(subtitle);
    }
    return header;
  }

  function _renderMeta(metaRows) {
    var ul = _el('ul', META_CLASS);
    for (var i = 0; i < metaRows.length; i++) {
      var row = metaRows[i];
      if (!_isPlainObject(row)) continue;
      var li = _el('li', META_ROW_CLASS);
      if (row.emphasis && row.emphasis !== 'none') {
        li.setAttribute('data-emphasis', _str(row.emphasis));
      }
      _setText(li, row.text || '');
      ul.appendChild(li);
    }
    return ul;
  }

  function _renderBadgeList(badges) {
    var ul = _el('ul', BADGES_CLASS);
    for (var i = 0; i < badges.length; i++) {
      var b = badges[i];
      if (!_isPlainObject(b)) continue;
      var li = _el('li', '');
      li.appendChild(renderBadge(b));
      ul.appendChild(li);
    }
    return ul;
  }

  function _renderMetricList(metrics) {
    var ul = _el('ul', METRICS_CLASS);
    for (var i = 0; i < metrics.length; i++) {
      var m = metrics[i];
      if (!_isPlainObject(m)) continue;
      ul.appendChild(renderMetric(m));
    }
    return ul;
  }

  // The painter emits action descriptors as DOM with full attribute
  // payload (`data-action-{kind,target-id,target-view,label,surface}`)
  // so a click delegator can reconstruct the ActionDescriptor without
  // looking the card up. The painter does NOT attach event listeners
  // (D37o-impl-5: behaviour wired by the renderer + selection bridge).
  function _renderActionList(actions, options) {
    void options;
    var ul = _el('ul', ACTIONS_CLASS);
    for (var i = 0; i < actions.length; i++) {
      var a = actions[i];
      if (!_isPlainObject(a)) continue;
      var li = _el('li', ACTION_CLASS);
      if (a.kind)       li.setAttribute('data-action-kind',        _str(a.kind));
      if (a.surface)    li.setAttribute('data-action-surface',     _str(a.surface));
      if (a.targetId)   li.setAttribute('data-action-target-id',   _str(a.targetId));
      if (a.targetView) li.setAttribute('data-action-target-view', _str(a.targetView));
      if (a.label)      li.setAttribute('data-action-label',       _str(a.label));
      li.setAttribute('role', 'button');
      _setText(li, a.label || a.kind || '');
      ul.appendChild(li);
    }
    return ul;
  }

  function _nodeActionContext(card, options) {
    var opts = options || {};
    var nodeKind = _str(card.kind || card.type || '');
    var nodeLabel = _str(card.name || card.label || card.title || card.id || 'node');
    return {
      lensId: _str(opts.lensId || card.lensId || card.lens || 'context'),
      nodeId: _str(card.id || ''),
      nodeKind: nodeKind,
      nodeLabel: nodeLabel,
      cardMetadata: card,
    };
  }

  function _hasNodeActions(context) {
    var graph = window.MIDASExplorerGraph || {};
    var registry = graph.nodeActionRegistry;
    if (!registry || typeof registry.hasActions !== 'function') return false;
    try { return registry.hasActions(context.lensId, context.nodeKind, context) === true; }
    catch (_) { return false; }
  }

  function _renderNodeActionTrigger(context, visible) {
    var button = _el('button', 'graph-node-action-trigger');
    button.type = 'button';
    button.setAttribute(NODE_ACTION_TRIGGER_ATTR, 'true');
    button.setAttribute('aria-label', 'More actions for ' + _str(context.nodeLabel || context.nodeId || 'node'));
    button.setAttribute('aria-haspopup', 'menu');
    button.setAttribute('aria-expanded', 'false');
    button.textContent = '\u2026';
    if (!visible) button.hidden = true;

    _on(button, 'pointerdown', function (event) {
      event.stopPropagation();
    });
    _on(button, 'mousedown', function (event) {
      event.stopPropagation();
    });
    _on(button, 'click', function (event) {
      event.preventDefault();
      event.stopPropagation();
      var graph = window.MIDASExplorerGraph || {};
      var menu = graph.nodeActionMenu;
      if (!menu || typeof menu.openForNode !== 'function') return;
      var ctx = {};
      var keys = Object.keys(context || {});
      for (var i = 0; i < keys.length; i++) ctx[keys[i]] = context[keys[i]];
      ctx.sourceEvent = event;
      menu.openForNode(button, ctx);
    });

    return button;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextCardPainter = {
    renderCard:     renderCard,
    renderCardBody: renderCardBody,
    renderBadge:    renderBadge,
    renderMetric:   renderMetric,
    _constants: {
      CARD_CLASS:         CARD_CLASS,
      KIND_CLASS_PREFIX:  KIND_CLASS_PREFIX,
      ROLE_CLASS_PREFIX:  ROLE_CLASS_PREFIX,
      BADGE_CLASS:        BADGE_CLASS,
      BADGE_CLASS_PREFIX: BADGE_CLASS_PREFIX,
      METRIC_CLASS:       METRIC_CLASS,
      ACTION_CLASS:       ACTION_CLASS,
      NODE_ACTION_TRIGGER_ATTR: NODE_ACTION_TRIGGER_ATTR,
    },
  };
})();
