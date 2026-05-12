// /explorer/assets/js/util-time.js — D27j-ui-foundation-3
//
// Pure date/time helpers extracted from the Explorer inline IIFE.
// Function bodies are byte-identical to the originals; only their
// physical location has moved.

(function () {
  'use strict';

  window.MIDASExplorerUtils = window.MIDASExplorerUtils || {};

  function formatRecordTimestamp(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    try {
      return d.toLocaleString();
    } catch {
      return iso;
    }
  }

  window.MIDASExplorerUtils.formatRecordTimestamp = formatRecordTimestamp;
})();
