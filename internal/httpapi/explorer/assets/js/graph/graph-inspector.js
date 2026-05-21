// /explorer/assets/js/graph/graph-inspector.js — D32a-impl-4
//
// Lens-agnostic Graph inspector at
// window.MIDASExplorerGraph.inspector.
//
// D32a-impl-4 expands the surface to own the reusable inspector
// frame setters that previously lived inline: name, generic field
// rows, summary rows, governance section html, and the bottom-panel
// actions list (record-navigation + reframe). Lens adapters call
// these helpers; Context-specific content rendering lives in
// graph/context/context-graph-inspector.js (D32a-impl-4).
//
// External dependencies:
//   window.MIDASExplorerUtils.escHtml — for safe html escaping
//   window.MIDASExplorerGraph._actionDispatcher (inline hook) —
//     wired by index.html's inline IIFE; receives action objects from
//     the Action button click handlers.
//
// Public surface on window.MIDASExplorerGraph.inspector:
//   show(mount) / hide(mount)   — lens-agnostic frame toggles
//   setName(name)               — write #gmap-details-name textContent
//   setFields(rows)             — render #gmap-details-fields rows
//   setSummary(rows)            — render #gmap-details-summary rows
//   setGovernance(html)         — write #gmap-details-governance innerHTML
//   setActions(actions)         — render #gmap-details-actions buttons
//   setInlineActions(node, actions)
//                               — render reframe button(s) on a node
//                                 card's inline-actions slot
//
// D37p-clean-2 — Dead Inspector Dispatcher Retirement.
//
// The pre-D37p-clean-2 module also exposed three dispatch functions
// (`register(lens, impl)` / `renderNode(lens, node, mount)` /
// `clear(lens, mount)`) plus an internal `_impls` registry. The
// entire dispatch surface had zero runtime call-sites — every live
// consumer reaches the frame setters directly via
// `MIDASExplorerGraph.inspector.set*` and reaches Authority's per-
// node renderer via `MIDASExplorerGraph.authorityInspector.selectNode`
// (or its `_rendererHooks.selectNode` analogue). The dispatch
// functions + registry are removed here; the lens-agnostic frame
// setters + show/hide remain because every existing inspector
// consumer continues to use them.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _dispatch() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._actionDispatcher) || null;
  }

  // ── Frame mount resolution (used by show / hide). ───────────────────
  //
  // D37p-clean-2 — `_resolveMount` previously also served the retired
  // `renderNode` / `clear` dispatch functions. With those gone, the
  // helper is still needed by show / hide so the inspector frame can
  // be toggled on the default `#gmap-details` mount without callers
  // having to thread the element.
  function _resolveMount(mount) {
    if (mount && typeof mount.appendChild === 'function') return mount;
    return document.getElementById('gmap-details') || document.body;
  }
  function show(mount) {
    var el = _resolveMount(mount);
    if (!el) return;
    el.classList.remove('is-hidden');
    el.removeAttribute('aria-hidden');
  }
  function hide(mount) {
    var el = _resolveMount(mount);
    if (!el) return;
    el.classList.add('is-hidden');
    el.setAttribute('aria-hidden', 'true');
  }

  // ── Frame setters — formerly inline (D32a-impl-3). ──────────────────
  function setName(name) {
    var el = document.getElementById('gmap-details-name');
    if (el) el.textContent = name;
  }
  function setFields(rows) {
    var el = document.getElementById('gmap-details-fields');
    if (!el) return;
    if (!Array.isArray(rows)) rows = [];
    el.innerHTML = rows.map(function (pair) {
      var k = pair[0], v = pair[1];
      return '<div class="gmap-details-row">' +
        '<span class="gmap-details-key">' + _escHtml(String(k)) + '</span>' +
        '<span class="gmap-details-val">' + _escHtml(v == null ? '—' : String(v)) + '</span>' +
      '</div>';
    }).join('');
  }
  function setSummary(rows) {
    var el = document.getElementById('gmap-details-summary');
    if (!el) return;
    if (!Array.isArray(rows)) rows = [];
    el.innerHTML = rows.map(function (pair) {
      var k = pair[0], v = pair[1];
      return '<div class="gmap-details-row">' +
        '<span class="gmap-details-key">' + _escHtml(String(k)) + '</span>' +
        '<span class="gmap-details-val">' + _escHtml(String(v)) + '</span>' +
      '</div>';
    }).join('');
  }
  function setGovernance(html) {
    var el = document.getElementById('gmap-details-governance');
    if (!el) return;
    el.innerHTML = html || '';
  }

  // ── Action buttons — bottom panel. ─────────────────────────────────
  // Whitelist of action kinds the bottom panel renders. Unknown kinds
  // are dropped silently. The action button's click handler invokes
  // the registered inline dispatcher (graph-shell wires this).
  function setActions(actions) {
    var el = document.getElementById('gmap-details-actions');
    if (!el) return;
    el.innerHTML = '';
    if (!Array.isArray(actions) || actions.length === 0) return;
    var dispatch = _dispatch();
    actions.forEach(function (action) {
      if (!action || typeof action !== 'object') return;
      if (action.kind === 'view-business-service-record' ||
          action.kind === 'view-capability-record') {
        if (!action.target_id) return;
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn btn-secondary gmap-action-view-record';
        btn.textContent = action.label || 'View record';
        btn.setAttribute('aria-label', (action.label || 'View record') + ' for ' + String(action.target_id));
        btn.addEventListener('click', function () { if (dispatch) dispatch(action); });
        el.appendChild(btn);
      } else if (action.kind === 'reframe-around-this') {
        if (!action.target_id || !action.target_view) return;
        var btn2 = document.createElement('button');
        btn2.type = 'button';
        btn2.className = 'btn btn-secondary gmap-action-reframe';
        btn2.textContent = action.label || 'Reframe around this';
        btn2.setAttribute('aria-label', (action.label || 'Reframe around this') + ' for ' + String(action.target_id));
        btn2.addEventListener('click', function () { if (dispatch) dispatch(action); });
        el.appendChild(btn2);
      }
    });
  }

  // ── Inline action buttons rendered into a node card's inline slot.
  // Only graph-primary kinds ('reframe-around-this') render inline.
  // Stops pointerdown/click propagation so the button doesn't reselect
  // or start a drag.
  function setInlineActions(node, actions) {
    var el = node && node.querySelector ? node.querySelector('.gmap-node-inline-actions') : null;
    if (!el) return;
    el.innerHTML = '';
    if (!Array.isArray(actions) || actions.length === 0) {
      el.setAttribute('hidden', '');
      return;
    }
    var dispatch = _dispatch();
    actions.forEach(function (action) {
      if (!action || typeof action !== 'object') return;
      if (action.kind !== 'reframe-around-this') return;
      if (!action.target_id || !action.target_view) return;
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'gmap-action-reframe-inline';
      btn.textContent = action.label || 'Reframe around this';
      btn.setAttribute('aria-label', (action.label || 'Reframe around this') + ' for ' + String(action.target_id));
      btn.addEventListener('pointerdown', function (e) { e.stopPropagation(); });
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        if (dispatch) dispatch(action);
      });
      el.appendChild(btn);
    });
    if (el.children.length === 0) el.setAttribute('hidden', '');
    else el.removeAttribute('hidden');
  }

  // D37p-clean-2 — Dispatcher methods (register / renderNode / clear)
  // retired; the live frame-setter surface stays because every existing
  // inspector consumer consumes it directly.
  window.MIDASExplorerGraph.inspector = {
    show:             show,
    hide:             hide,
    setName:          setName,
    setFields:        setFields,
    setSummary:       setSummary,
    setGovernance:    setGovernance,
    setActions:       setActions,
    setInlineActions: setInlineActions,
  };
})();
