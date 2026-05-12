// /explorer/assets/js/util-format.js — D27j-ui-foundation-3
//
// Pure value/HTML/comparison formatters extracted from the Explorer
// inline IIFE. Every function here is pure (no DOM, no fetch, no
// module-state dependency) and is exposed on the
// window.MIDASExplorerUtils namespace. The inline script binds these
// to local consts at the top of its IIFE so existing call-sites
// (escHtml is called ~120 times) continue to work without rewrite.
//
// Function bodies are byte-identical to the originals; only their
// physical location has moved.

(function () {
  'use strict';

  window.MIDASExplorerUtils = window.MIDASExplorerUtils || {};

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  function escHtml(s) {
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  // ── Comparison helpers ─────────────────────────────────────────────────────
  function deepEqual(a, b) {
    if (a === b) return true;
    if (a === null || b === null || a === undefined || b === undefined) return a === b;
    if (typeof a !== typeof b) return false;
    if (typeof a !== 'object') return a === b;
    if (Array.isArray(a) !== Array.isArray(b)) return false;
    const ka = Object.keys(a).sort();
    const kb = Object.keys(b).sort();
    if (ka.length !== kb.length || ka.join('\0') !== kb.join('\0')) return false;
    return ka.every(k => deepEqual(a[k], b[k]));
  }

  function formatVal(v) {
    if (v === null || v === undefined) return '—';
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }

  function formatFieldValue(v) {
    if (v == null || v === '') return '<span class="services-record-field-val muted">—</span>';
    return '<span class="services-record-field-val">' + escHtml(String(v)) + '</span>';
  }

  function formatExternalRef(ref) {
    if (!ref || typeof ref !== 'object') return '';
    const sys = ref.source_system || '';
    const id  = ref.source_id || '';
    if (sys && id) return sys + ':' + id;
    return sys || id || '(set)';
  }

  // formatAIBindingScope returns the binding's scope as `kind=id`, using the
  // same precedence chain as the connector-resolution code in
  // renderGovernanceMap (surface > process > capability > business_service).
  // A binding with NO scope id at all renders as `unscoped` rather than
  // throwing or returning an empty string.
  function formatAIBindingScope(b) {
    if (!b) return 'unscoped';
    if (b.surface_id)          return 'surface=' + b.surface_id;
    if (b.process_id)          return 'process=' + b.process_id;
    if (b.capability_id)       return 'capability=' + b.capability_id;
    if (b.business_service_id) return 'business_service=' + b.business_service_id;
    return 'unscoped';
  }

  // formatAIBindingDetail composes role + scope + description into one line
  // for a single binding row. Each part is dropped when absent so the
  // resulting string never carries empty pieces or trailing separators.
  function formatAIBindingDetail(b) {
    if (!b) return 'unscoped';
    const parts = [];
    if (b.role) parts.push('role=' + b.role);
    parts.push(formatAIBindingScope(b));
    if (b.description) parts.push(b.description);
    return parts.join(' · ');
  }

  function formatConsequenceVal(type, amount, currency) {
    if (!type) return '—';
    if (type === 'monetary') {
      if (amount == null) return type;
      const sym = currency === 'GBP' ? '£' : (currency ? currency + ' ' : '');
      return `${sym}${Number(amount).toLocaleString()}`;
    }
    if (type === 'risk_rating') return amount != null ? String(amount) : type;
    return type;
  }

  function formatGmapDemoValue(v, unit) {
    if (unit === '%') return v.toFixed(1) + '%';
    return Math.round(v).toString();
  }

  function recordsOutcomeClass(o) {
    switch (o) {
      case 'ACCEPT':
      case 'ALLOW':
      case 'APPROVE':
        return 'accept';
      case 'ESCALATE':
        return 'escalate';
      case 'REJECT':
      case 'DENY':
        return 'reject';
      case 'CLARIFY':
        return 'clarify';
      default:
        return '';
    }
  }

  function bandClassFor(band) {
    if (band === 'Stable' || band === 'No exposure') return 'gmap-evidence-tray-tile-status-stable';
    if (band === 'Critical')                          return 'gmap-evidence-tray-tile-status-critical';
    if (band === 'Watch' || band === 'Drifting')      return 'gmap-evidence-tray-tile-status-drifting';
    return '';
  }

  // getTruncationInfo computes the (rendered, total, omitted) triple for
  // a single layer. Defensive against non-finite or negative inputs.
  function getTruncationInfo(total, rendered) {
    const safeTotal = Math.max(0, Number.isFinite(total) ? total : 0);
    const safeRendered = Math.max(0, Number.isFinite(rendered) ? rendered : 0);
    return {
      total: safeTotal,
      rendered: safeRendered,
      omitted: Math.max(0, safeTotal - safeRendered),
    };
  }

  window.MIDASExplorerUtils.escapeHTML            = escapeHTML;
  window.MIDASExplorerUtils.escHtml               = escHtml;
  window.MIDASExplorerUtils.deepEqual             = deepEqual;
  window.MIDASExplorerUtils.formatVal             = formatVal;
  window.MIDASExplorerUtils.formatFieldValue      = formatFieldValue;
  window.MIDASExplorerUtils.formatExternalRef     = formatExternalRef;
  window.MIDASExplorerUtils.formatAIBindingScope  = formatAIBindingScope;
  window.MIDASExplorerUtils.formatAIBindingDetail = formatAIBindingDetail;
  window.MIDASExplorerUtils.formatConsequenceVal  = formatConsequenceVal;
  window.MIDASExplorerUtils.formatGmapDemoValue   = formatGmapDemoValue;
  window.MIDASExplorerUtils.recordsOutcomeClass   = recordsOutcomeClass;
  window.MIDASExplorerUtils.bandClassFor          = bandClassFor;
  window.MIDASExplorerUtils.getTruncationInfo     = getTruncationInfo;
})();
