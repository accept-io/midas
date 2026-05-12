// /explorer/assets/js/util-dom.js — D27j-ui-foundation-3
//
// Parameter-driven DOM mutation helpers extracted from the Explorer
// inline IIFE. These helpers do not look up DOM elements themselves —
// they receive the target element as an argument from the caller, so
// they have no hidden dependency on DOM IDs or app state.
//
// Function bodies are byte-identical to the originals; only their
// physical location has moved.

(function () {
  'use strict';

  window.MIDASExplorerUtils = window.MIDASExplorerUtils || {};

  function copyToClipboard(text, btn) {
    navigator.clipboard.writeText(text).then(() => {
      const orig = btn.textContent;
      btn.textContent = 'Copied!';
      btn.classList.add('copied');
      setTimeout(() => { btn.textContent = orig; btn.classList.remove('copied'); }, 1500);
    }).catch(() => {
      btn.textContent = 'Failed';
      setTimeout(() => { btn.textContent = 'Copy'; }, 1500);
    });
  }

  window.MIDASExplorerUtils.copyToClipboard = copyToClipboard;
})();
