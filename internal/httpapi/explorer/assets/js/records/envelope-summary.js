// /explorer/assets/js/records/envelope-summary.js — D27j-ui-foundation-6
//
// Pure Records helpers extracted from the inline IIFE. Both helpers
// are parameter-driven: they read no DOM, mutate no module state,
// fetch nothing, and call no render functions. The two helpers
// reference recordsOutcomeClass and formatRecordTimestamp from the
// MIDASExplorerUtils namespace (D27j-ui-foundation-3); this file
// loads AFTER the util-*.js scripts, so those bindings are present
// at IIFE-load time.
//
// Function bodies are byte-identical to the originals; only their
// physical location has moved.

(function () {
  'use strict';

  window.MIDASExplorerRecords = window.MIDASExplorerRecords || {};

  const Utils = window.MIDASExplorerUtils || {};
  const recordsOutcomeClass   = Utils.recordsOutcomeClass;
  const formatRecordTimestamp = Utils.formatRecordTimestamp;

  // Map a D26a envelopeSummary { id, state, outcome, reason_code,
  // request_source, surface_id, business_service_id, agent_id,
  // created_at, evaluated_at, ... } into the existing row shape
  // consumed by renderRecordsTable / renderRecordsDetail. Optional
  // fields fall back to the empty string ('') so the renderer always
  // has a string to display — never undefined.
  function mapExplorerEnvelopeToRecordRow(item) {
    if (!item || typeof item !== 'object') return null;
    const outcomeRaw  = String(item.outcome || '').toUpperCase();
    const outcomeText = outcomeRaw || '—';
    const outcomeCls  = recordsOutcomeClass(outcomeRaw);
    const tsIso       = item.evaluated_at || item.created_at || '';
    return {
      id:            String(item.id || ''),
      state:         String(item.state || ''),
      outcome:       outcomeText,
      outcomeClass:  outcomeCls,
      reason:        String(item.reason_code || ''),
      requestSource: String(item.request_source || ''),
      surface:       String(item.surface_id || ''),
      agent:         String(item.agent_id || ''),
      bs:            String(item.business_service_id || ''),
      processId:     String(item.process_id || ''),
      profileId:     String(item.profile_id || ''),
      grantId:       String(item.grant_id || ''),
      createdAt:     String(item.created_at || ''),
      evaluatedAt:   String(item.evaluated_at || ''),
      timeIso:       String(tsIso),
      time:          formatRecordTimestamp(tsIso),
    };
  }

  // computeRecordsRuntimeMetrics counts the relevant outcomes case-
  // insensitively across an array of mapped or raw rows. Defensive
  // against non-array input and missing/empty outcome fields.
  //
  // Outcome literal mapping: MIDAS evaluator emits 'accept' / 'escalate'
  // / 'reject' / 'request_clarification' (see internal/eval/outcome.go).
  // The brief uses the labels approve / clarify / stop, so the helper
  // accepts both spellings — defensive against future evaluator
  // additions and consistent with the disclaimer in the brief about
  // handling casing defensively. 'stop' is a forward-looking outcome
  // reserved for governance kill-switches; it counts to zero today.
  function computeRecordsRuntimeMetrics(rows) {
    const m = { total: 0, approved: 0, escalated: 0, rejected: 0, clarify: 0, stopped: 0 };
    if (!Array.isArray(rows)) return m;
    for (let i = 0; i < rows.length; i++) {
      const r = rows[i];
      if (!r) continue;
      m.total += 1;
      // The mapper uppercases r.outcome, but the raw envelope outcome is
      // lowercase. Normalise defensively so this helper is stable
      // whether it is called with mapped rows or raw D26a items.
      const o = String(r.outcome || '').toLowerCase();
      if (o === 'approve' || o === 'accept') {
        m.approved += 1;
      } else if (o === 'escalate') {
        m.escalated += 1;
      } else if (o === 'reject') {
        m.rejected += 1;
      } else if (o === 'clarify' || o === 'request_clarification') {
        m.clarify += 1;
      } else if (o === 'stop') {
        m.stopped += 1;
      }
      // Unknown / empty outcomes only count toward the total — this
      // avoids misclassifying envelopes that have not yet completed
      // evaluation (state=evaluating with a missing outcome).
    }
    return m;
  }

  window.MIDASExplorerRecords.mapExplorerEnvelopeToRecordRow = mapExplorerEnvelopeToRecordRow;
  window.MIDASExplorerRecords.computeRecordsRuntimeMetrics   = computeRecordsRuntimeMetrics;
})();
