// /explorer/assets/js/records/evidence-packet.js — D30h
//
// Runtime Evidence Packet and Integrity UI controller. Powers the
// "Verify integrity" and "View evidence packet" buttons in the
// Records detail rail by calling the production endpoints:
//
//   GET /v1/evidence/envelopes/{id}/integrity
//   GET /v1/evidence/envelopes/{id}/packet
//
// Read-only by construction: this module never issues a mutating
// HTTP method, never downloads files, never signs or alters packet
// content. The packet endpoint returns the envelope + audit-event
// chain + integrity result; rendering is summary + selectable
// pretty-printed JSON.
//
// Modular-monolith posture: self-contained IIFE registering
// window.MIDASExplorerRecords.evidencePacket. If runtime evidence
// later extracts to a dedicated frontend bundle, this file is the
// unit that moves alongside evidence-search.js.

(function () {
  'use strict';

  window.MIDASExplorerRecords = window.MIDASExplorerRecords || {};

  // -------------------------------------------------------------------------
  // Constants — DOM ids and exact state copy.
  // -------------------------------------------------------------------------

  var IDS = {
    detailBody:    'records-detail-body',
    detailId:      'records-detail-id',
    verifyBtn:     'records-verify-integrity-btn',
    packetBtn:     'records-view-packet-btn',
    copyPacketBtn: 'records-copy-packet-btn',
    integritySlot: 'records-integrity-panel',
    packetSlot:    'records-packet-panel',
    packetJSON:    'records-packet-json',
  };

  // Exact strings — pinned by explorer_evidence_packet_test.go.
  var COPY = {
    noEnvelope:        'No envelope selected.',
    integrityLoading:  'Verifying evidence integrity…',
    integrityValid:    'Audit chain verified.',
    integrityInvalid:  'Audit chain integrity issue detected.',
    integrityError:    'Integrity status could not be loaded.',
    packetLoading:     'Loading evidence packet…',
    packetLoaded:      'Evidence packet loaded.',
    packetError:       'Evidence packet could not be loaded.',
  };

  // -------------------------------------------------------------------------
  // Local HTML-escape (defensive, matches the style used by
  // evidence-search.js and audit-event-renderers.js).
  // -------------------------------------------------------------------------

  function escHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  // -------------------------------------------------------------------------
  // Helpers.
  // -------------------------------------------------------------------------

  // envelopeIDFromDOM reads the currently selected envelope id from
  // the detail rail header. Returns '' when no record is selected
  // (placeholder '—' in the header element).
  function envelopeIDFromDOM() {
    var el = document.getElementById(IDS.detailId);
    if (!el) return '';
    var t = (el.textContent || '').trim();
    if (!t || t === '—') return '';
    return t;
  }

  // fetchJSON issues a GET against url and resolves with the parsed
  // body. Throws on non-2xx so callers can surface their own error
  // copy. credentials: 'same-origin' carries the existing Explorer
  // session cookie; identical pattern to evidence-search.js.
  function fetchJSON(url) {
    return fetch(url, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json' },
    }).then(function (resp) {
      if (!resp.ok) {
        throw new Error('non-2xx status ' + resp.status);
      }
      return resp.json();
    });
  }

  // kvRow renders a single key/value row inside the integrity or
  // packet summary block. Mono variant marks identifier-shaped
  // values (hashes, ids, timestamps).
  function kvRow(label, value, mono) {
    var v = (value == null || value === '') ? '—' : String(value);
    var valueHTML = mono
      ? '<code class="runtime-evidence-packet-mono">' + escHtml(v) + '</code>'
      : escHtml(v);
    return (
      '<div class="runtime-evidence-integrity-kv">' +
        '<span class="runtime-evidence-integrity-kv-key">' + escHtml(label) + '</span>' +
        '<span class="runtime-evidence-integrity-kv-value">' + valueHTML + '</span>' +
      '</div>'
    );
  }

  // -------------------------------------------------------------------------
  // Integrity panel.
  // -------------------------------------------------------------------------

  // renderIntegrity populates the integrity slot from the
  // RuntimeEvidenceIntegrityResponse shape D30d returns. Valid and
  // invalid are both HTTP 200; this function never treats valid:false
  // as a transport error.
  function renderIntegrity(result) {
    var slot = document.getElementById(IDS.integritySlot);
    if (!slot) return;
    if (!result || typeof result !== 'object') {
      slot.innerHTML = '<p class="runtime-evidence-packet-error">' +
        escHtml(COPY.integrityError) + '</p>';
      return;
    }
    var valid = result.valid === true;
    var statusClass = valid ? 'is-valid' : 'is-invalid';
    var statusText  = valid ? COPY.integrityValid : COPY.integrityInvalid;
    var rows =
      kvRow('Chain length',     result.chain_length) +
      kvRow('Checked at',       result.checked_at, true) +
      kvRow('First event hash', result.first_event_hash, true) +
      kvRow('Final event hash', result.final_event_hash, true);
    if (!valid) {
      if (result.error_kind) {
        rows += kvRow('Error kind', result.error_kind);
      }
      if (result.error_message) {
        rows += kvRow('Error message', result.error_message);
      }
    }
    slot.innerHTML =
      '<p class="runtime-evidence-integrity-status ' + statusClass + '">' +
        escHtml(statusText) +
      '</p>' +
      '<div class="runtime-evidence-integrity-kv-list">' + rows + '</div>';
  }

  // loadIntegrity fetches and renders the integrity result for the
  // supplied envelope id. envelopeID == '' renders a not-selected
  // message and skips the fetch.
  function loadIntegrity(envelopeID) {
    var slot = document.getElementById(IDS.integritySlot);
    if (!slot) return Promise.resolve();
    if (!envelopeID) {
      slot.innerHTML = '<p class="runtime-evidence-packet-state">' +
        escHtml(COPY.noEnvelope) + '</p>';
      return Promise.resolve();
    }
    slot.innerHTML = '<p class="runtime-evidence-packet-state">' +
      escHtml(COPY.integrityLoading) + '</p>';
    var url = '/v1/evidence/envelopes/' + encodeURIComponent(envelopeID) + '/integrity';
    return fetchJSON(url).then(renderIntegrity).catch(function () {
      slot.innerHTML = '<p class="runtime-evidence-packet-error">' +
        escHtml(COPY.integrityError) + '</p>';
    });
  }

  // -------------------------------------------------------------------------
  // Packet panel.
  // -------------------------------------------------------------------------

  // renderPacket populates the packet slot from the
  // RuntimeEvidencePacket shape D30e returns. Renders a summary line,
  // a small kv block (envelope id, generated at, audit-event count,
  // integrity status), then the full JSON in a selectable <pre>. The
  // <pre> is populated via textContent so even pathological payload
  // strings cannot escape the block.
  function renderPacket(packet) {
    var slot = document.getElementById(IDS.packetSlot);
    if (!slot) return;
    if (!packet || typeof packet !== 'object') {
      slot.innerHTML = '<p class="runtime-evidence-packet-error">' +
        escHtml(COPY.packetError) + '</p>';
      return;
    }
    var integrity = (packet.integrity && typeof packet.integrity === 'object')
      ? packet.integrity : {};
    var auditCount = (packet.audit_events && packet.audit_events.length != null)
      ? packet.audit_events.length : 0;
    var integrityCell = integrity.valid === true
      ? 'verified'
      : ('issue detected' + (integrity.error_kind
            ? ' (' + integrity.error_kind + ')'
            : ''));

    var summaryRows =
      kvRow('Envelope ID',  packet.envelope_id, true) +
      kvRow('Generated at', packet.generated_at, true) +
      kvRow('Audit events', auditCount) +
      kvRow('Integrity',    integrityCell);

    slot.innerHTML =
      '<p class="runtime-evidence-packet-state">' + escHtml(COPY.packetLoaded) + '</p>' +
      '<div class="runtime-evidence-packet-summary">' + summaryRows + '</div>' +
      '<div class="runtime-evidence-packet-actions">' +
        '<button type="button" class="records-resource-action runtime-evidence-action-button"' +
          ' id="' + escHtml(IDS.copyPacketBtn) + '">' +
          '<span>Copy packet JSON</span>' +
        '</button>' +
      '</div>' +
      '<pre class="runtime-evidence-packet-json"' +
        ' id="' + escHtml(IDS.packetJSON) + '"></pre>';

    var pre = document.getElementById(IDS.packetJSON);
    if (pre) {
      try {
        pre.textContent = JSON.stringify(packet, null, 2);
      } catch (e) {
        pre.textContent = '';
      }
    }
    wireCopyPacketButton();
  }

  // wireCopyPacketButton attaches the existing
  // MIDASExplorerUtils.copyToClipboard helper to the Copy packet JSON
  // button. The button only exists after a successful packet render;
  // re-rendering replaces the button element, so a fresh listener
  // wires onto the fresh element.
  function wireCopyPacketButton() {
    var btn = document.getElementById(IDS.copyPacketBtn);
    var pre = document.getElementById(IDS.packetJSON);
    if (!btn || !pre) return;
    var utils = window.MIDASExplorerUtils || null;
    if (!utils || typeof utils.copyToClipboard !== 'function') return;
    btn.addEventListener('click', function (ev) {
      ev.preventDefault();
      utils.copyToClipboard(pre.textContent || '', btn);
    });
  }

  // loadPacket fetches and renders the packet for the supplied
  // envelope id. envelopeID == '' renders a not-selected message and
  // skips the fetch.
  function loadPacket(envelopeID) {
    var slot = document.getElementById(IDS.packetSlot);
    if (!slot) return Promise.resolve();
    if (!envelopeID) {
      slot.innerHTML = '<p class="runtime-evidence-packet-state">' +
        escHtml(COPY.noEnvelope) + '</p>';
      return Promise.resolve();
    }
    slot.innerHTML = '<p class="runtime-evidence-packet-state">' +
      escHtml(COPY.packetLoading) + '</p>';
    var url = '/v1/evidence/envelopes/' + encodeURIComponent(envelopeID) + '/packet';
    return fetchJSON(url).then(renderPacket).catch(function () {
      slot.innerHTML = '<p class="runtime-evidence-packet-error">' +
        escHtml(COPY.packetError) + '</p>';
    });
  }

  // clear empties both panels. Called externally when the operator
  // wants to reset the state explicitly; the per-render lifecycle in
  // index.html already produces empty panels on every record
  // selection because the slots are part of bodyEl.innerHTML.
  function clear() {
    var i = document.getElementById(IDS.integritySlot);
    if (i) i.innerHTML = '';
    var p = document.getElementById(IDS.packetSlot);
    if (p) p.innerHTML = '';
  }

  // -------------------------------------------------------------------------
  // Wiring.
  // -------------------------------------------------------------------------

  // init attaches one delegated click handler to the detail rail
  // body. Because renderRecordsDetail() regenerates the rail's
  // innerHTML on every selection, button elements are recreated each
  // time; delegation keeps a single, stable listener at the parent
  // level. The dataset flag guards against double-wiring if init()
  // is called more than once.
  function init() {
    var body = document.getElementById(IDS.detailBody);
    if (!body || body.dataset.runtimeEvidencePacketWired === 'true') return;
    body.addEventListener('click', function (ev) {
      var t = ev.target;
      if (!t) return;
      // Click targets may be nested glyphs; walk up to the button.
      while (t && t !== body && (!t.id || (t.id !== IDS.verifyBtn && t.id !== IDS.packetBtn))) {
        t = t.parentNode;
      }
      if (!t || t === body) return;
      if (t.id === IDS.verifyBtn) {
        ev.preventDefault();
        loadIntegrity(envelopeIDFromDOM());
      } else if (t.id === IDS.packetBtn) {
        ev.preventDefault();
        loadPacket(envelopeIDFromDOM());
      }
    });
    body.dataset.runtimeEvidencePacketWired = 'true';
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // -------------------------------------------------------------------------
  // Public namespace.
  // -------------------------------------------------------------------------

  window.MIDASExplorerRecords.evidencePacket = {
    init:          init,
    loadIntegrity: loadIntegrity,
    loadPacket:    loadPacket,
    clear:         clear,
    IDS:  IDS,
    COPY: COPY,
  };
})();
