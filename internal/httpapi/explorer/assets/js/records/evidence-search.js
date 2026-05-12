// /explorer/assets/js/records/evidence-search.js — D30g + D30k
//
// Runtime Evidence Search controller. Powers the search panel in the
// Records view by calling the production cross-envelope audit-event
// search endpoint:
//
//   GET /v1/evidence/audit-events
//
// Read-only by construction: this module never issues a mutating HTTP
// method, never reads or writes localStorage outside the existing
// Explorer conventions, and never duplicates FailModePolicy rendering
// — it reuses MIDASExplorerRecords.auditEventRenderers.renderAuditEventCard
// through the public namespace.
//
// D30k layers cursor-pagination on top of D30g. The backend cursor
// returned by /v1/evidence/audit-events (D30j) is treated as an
// opaque string here: never decoded, parsed, base64-stripped, or
// inspected. The module captures the URLSearchParams used for the
// first-page search and reuses that snapshot — not live form
// values — for every Load more click, so a mid-pagination filter
// edit cannot silently leak into the accumulating result set.
//
// Modular-monolith posture: the module is a self-contained IIFE under
// window.MIDASExplorerRecords.evidenceSearch. If evidence search later
// extracts to a dedicated frontend bundle, this file is the unit that
// moves.

(function () {
  'use strict';

  window.MIDASExplorerRecords = window.MIDASExplorerRecords || {};

  // -------------------------------------------------------------------------
  // Constants — DOM ids the markup ships with, exact state copy strings.
  // -------------------------------------------------------------------------

  // Element ids declared in index.html for the search panel. Centralised so
  // markup + JS stay in sync.
  var IDS = {
    panel:        'runtime-evidence-search',
    form:         'runtime-evidence-search-form',
    fieldEventType:     'runtime-evidence-search-event-type',
    fieldEventTypes:    'runtime-evidence-search-event-types',
    fieldEnvelopeID:    'runtime-evidence-search-envelope-id',
    fieldRequestSource: 'runtime-evidence-search-request-source',
    fieldRequestID:     'runtime-evidence-search-request-id',
    fieldSince:         'runtime-evidence-search-since',
    fieldUntil:         'runtime-evidence-search-until',
    fieldLimit:         'runtime-evidence-search-limit',
    fieldOrder:         'runtime-evidence-search-order',
    buttonSearch:       'runtime-evidence-search-button',
    buttonClear:        'runtime-evidence-search-clear-button',
    buttonMore:         'runtime-evidence-search-more-btn',
    state:        'runtime-evidence-search-state',
    results:      'runtime-evidence-search-results',
    more:         'runtime-evidence-search-more',
    moreState:    'runtime-evidence-search-more-state',
  };

  // State copy — exact strings, pinned by explorer_evidence_search_test.go.
  // Changing these is a deliberate UX decision and must update the test pins
  // at the same time. D30k adds the loadingMore + errorMore variants so the
  // Load more affordance has its own messaging that does not clobber the
  // existing results.
  var COPY = {
    initial:     'Search runtime evidence across evaluation envelopes.',
    loading:     'Searching runtime evidence…',
    loadingMore: 'Loading more runtime evidence…',
    empty:       'No audit events matched the current filters.',
    error:       'Runtime evidence search could not be loaded.',
    errorMore:   'More runtime evidence could not be loaded.',
  };

  // Endpoint — production search route.
  var ENDPOINT = '/v1/evidence/audit-events';

  // -------------------------------------------------------------------------
  // Module-scoped state (D30k).
  //
  // currentQueryParams holds the URLSearchParams snapshot captured at
  // the start of the most recent first-page search. Every Load more
  // click reuses this snapshot — the module never re-reads live form
  // values for follow-up pages — so a mid-pagination filter edit
  // cannot silently leak into the accumulating result set. Editing
  // filters and pressing Search starts a new result set, which
  // overwrites this snapshot.
  //
  // nextCursor is the opaque pagination token returned by the previous
  // response. The module never decodes / parses / inspects this
  // string — it round-trips verbatim to the next request. The empty
  // string is the "no further pages" sentinel.
  // -------------------------------------------------------------------------
  var currentQueryParams = null;
  var nextCursor = '';

  // -------------------------------------------------------------------------
  // Pure helpers (exported for tests where useful).
  // -------------------------------------------------------------------------

  // valueOf reads a form input's trimmed value. Returns empty string when
  // the element is missing or its value is whitespace.
  function valueOf(id) {
    var el = document.getElementById(id);
    if (!el) return '';
    return String(el.value == null ? '' : el.value).trim();
  }

  // setValue assigns a value to a form input. No-op when the element is
  // missing.
  function setValue(id, v) {
    var el = document.getElementById(id);
    if (el) el.value = v == null ? '' : v;
  }

  // splitEventTypes parses the event_types CSV input. Trims whitespace
  // around each token and drops empty tokens. Returns an array of strings
  // (possibly empty). Empty tokens are dropped client-side rather than
  // forwarded — the backend rejects them with 400 and dropping locally
  // gives a cleaner request shape.
  function splitEventTypes(raw) {
    if (!raw) return [];
    var out = [];
    var parts = String(raw).split(',');
    for (var i = 0; i < parts.length; i++) {
      var tok = parts[i].trim();
      if (tok.length > 0) out.push(tok);
    }
    return out;
  }

  // buildQuery constructs a URLSearchParams from form values. Empty values
  // are omitted entirely — the backend applies its own defaults for
  // limit/order when the parameter is absent. event_types wins over
  // event_type at the wire level when non-empty, but the frontend forwards
  // both as the operator entered them.
  //
  // buildQuery deliberately does NOT emit a cursor parameter. The cursor
  // is a D30k follow-up concern, attached only by loadMore() so the
  // captured first-page params remain reusable as-is.
  function buildQuery(formValues) {
    var p = new URLSearchParams();
    if (formValues.eventType)     p.set('event_type',     formValues.eventType);
    if (formValues.eventTypes && formValues.eventTypes.length > 0) {
      p.set('event_types', formValues.eventTypes.join(','));
    }
    if (formValues.envelopeID)    p.set('envelope_id',    formValues.envelopeID);
    if (formValues.requestSource) p.set('request_source', formValues.requestSource);
    if (formValues.requestID)     p.set('request_id',     formValues.requestID);
    if (formValues.since)         p.set('since',          formValues.since);
    if (formValues.until)         p.set('until',          formValues.until);
    if (formValues.limit)         p.set('limit',          formValues.limit);
    if (formValues.order)         p.set('order',          formValues.order);
    return p;
  }

  // collectFormValues reads every supported filter from the DOM and
  // returns a normalised object. Tests do not call this directly — they
  // exercise the search via the public run function — but having it
  // factored out keeps run() small.
  function collectFormValues() {
    return {
      eventType:     valueOf(IDS.fieldEventType),
      eventTypes:    splitEventTypes(valueOf(IDS.fieldEventTypes)),
      envelopeID:    valueOf(IDS.fieldEnvelopeID),
      requestSource: valueOf(IDS.fieldRequestSource),
      requestID:     valueOf(IDS.fieldRequestID),
      since:         valueOf(IDS.fieldSince),
      until:         valueOf(IDS.fieldUntil),
      limit:         valueOf(IDS.fieldLimit),
      order:         valueOf(IDS.fieldOrder),
    };
  }

  // -------------------------------------------------------------------------
  // DOM rendering helpers.
  // -------------------------------------------------------------------------

  // setStateMessage replaces the state container with a single message.
  // kind is one of 'initial' | 'loading' | 'empty' | 'error' and selects
  // a modifier class for styling.
  function setStateMessage(kind, message) {
    var stateEl = document.getElementById(IDS.state);
    if (!stateEl) return;
    var classes = 'runtime-evidence-search-state runtime-evidence-search-state-' + kind;
    stateEl.className = classes;
    stateEl.textContent = message;
    stateEl.hidden = false;
  }

  // clearStateMessage hides the state container so result cards can take
  // its place without an intervening message line.
  function clearStateMessage() {
    var stateEl = document.getElementById(IDS.state);
    if (!stateEl) return;
    stateEl.textContent = '';
    stateEl.hidden = true;
  }

  // setLoadMoreState writes a message next to the Load more button.
  // Distinct from setStateMessage so the existing accumulated results
  // remain on screen during a follow-up page load or load-more error.
  function setLoadMoreState(kind, message) {
    var el = document.getElementById(IDS.moreState);
    if (!el) return;
    el.className = 'runtime-evidence-search-more-state runtime-evidence-search-more-state-' + kind;
    el.textContent = message;
  }

  function clearLoadMoreState() {
    var el = document.getElementById(IDS.moreState);
    if (!el) return;
    el.className = 'runtime-evidence-search-more-state';
    el.textContent = '';
  }

  // showLoadMore / hideLoadMore toggle the wrapper's hidden attribute.
  // The wrapper carries the data-runtime-evidence-search-more attribute
  // so future styling / E2E selectors do not depend on the id.
  function showLoadMore() {
    var wrap = document.getElementById(IDS.more);
    if (wrap) wrap.hidden = false;
  }

  function hideLoadMore() {
    var wrap = document.getElementById(IDS.more);
    if (wrap) wrap.hidden = true;
    clearLoadMoreState();
  }

  // setButtonDisabled toggles the disabled attribute on a button. No-op
  // when the button is missing so the helper is safe to call in init
  // paths and partial-DOM tests.
  function setButtonDisabled(id, disabled) {
    var btn = document.getElementById(id);
    if (btn) btn.disabled = !!disabled;
  }

  // renderResults replaces the results container with one card per
  // returned event. Used for first-page searches; Load more uses
  // appendResults to preserve the existing cards.
  function renderResults(items) {
    var resultsEl = document.getElementById(IDS.results);
    if (!resultsEl) return;
    resultsEl.innerHTML = buildCardsHTML(items);
  }

  // appendResults appends new cards to the existing results without
  // re-rendering prior cards. Used by Load more so accumulated results
  // and user-visible order are preserved across cursor-paginated
  // pages.
  function appendResults(items) {
    var resultsEl = document.getElementById(IDS.results);
    if (!resultsEl) return;
    var html = buildCardsHTML(items);
    if (html) resultsEl.insertAdjacentHTML('beforeend', html);
  }

  // buildCardsHTML renders an array of audit events to an HTML string.
  // Reuses the Records-rail renderer through its public namespace —
  // no FailModePolicy logic is duplicated here. Each result is wrapped
  // in a .runtime-evidence-search-result-card for search-specific
  // styling and to give a stable selector for tests / future
  // deep-linking.
  function buildCardsHTML(items) {
    var renderers = (window.MIDASExplorerRecords &&
                     window.MIDASExplorerRecords.auditEventRenderers) || null;
    if (!renderers || typeof renderers.renderAuditEventCard !== 'function') {
      return '';
    }
    var html = '';
    for (var i = 0; i < items.length; i++) {
      var ev = items[i];
      if (!ev || typeof ev !== 'object') continue;
      var card = '';
      try {
        card = renderers.renderAuditEventCard(ev) || '';
      } catch (e) {
        card = '';
      }
      var envelopeID = String(ev.envelope_id == null ? '' : ev.envelope_id);
      html +=
        '<article class="runtime-evidence-search-result-card"' +
          ' data-envelope-id="' + escAttr(envelopeID) + '">' +
          card +
        '</article>';
    }
    return html;
  }

  // clearResults empties the results container. Used by Clear and at the
  // start of a new search.
  function clearResults() {
    var resultsEl = document.getElementById(IDS.results);
    if (resultsEl) resultsEl.innerHTML = '';
  }

  // escAttr performs minimal HTML-attribute escaping. Defensive even
  // though envelope_id values come from the API and are constrained.
  function escAttr(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  // -------------------------------------------------------------------------
  // Fetch + run.
  // -------------------------------------------------------------------------

  // run executes one first-page search cycle: collect the form, build
  // the query, fetch, render. D30k additions: capture the query params
  // snapshot for cursor-driven follow-ups, reset the accumulated
  // nextCursor + Load more affordance, and surface a Load more button
  // when the response carries a next_cursor.
  //
  // Returns a Promise so tests can await it. Errors are surfaced via
  // the state message; the Promise resolves successfully in all
  // branches because the UX-level error display is the load-bearing
  // signal.
  function run() {
    var values = collectFormValues();
    var query = buildQuery(values);

    // D30k: snapshot the params for follow-up Load more calls and
    // reset the accumulator state. A new first-page search overrides
    // any prior cursor.
    currentQueryParams = query;
    nextCursor = '';
    hideLoadMore();

    var url = ENDPOINT;
    var qs = query.toString();
    if (qs.length > 0) url = url + '?' + qs;

    setStateMessage('loading', COPY.loading);
    clearResults();
    setButtonDisabled(IDS.buttonSearch, true);
    setButtonDisabled(IDS.buttonMore, true);

    return fetch(url, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json' },
    }).then(function (resp) {
      if (!resp.ok) {
        throw new Error('non-2xx status ' + resp.status);
      }
      return resp.json();
    }).then(function (body) {
      var items = (body && Array.isArray(body.items)) ? body.items : [];
      // D30k: next_cursor is omitted by the server when no further
      // pages exist, so an absent field == empty string here. The
      // string is treated as opaque — no decoding, no parsing.
      var cursor = (body && typeof body.next_cursor === 'string') ? body.next_cursor : '';
      if (items.length === 0) {
        clearResults();
        setStateMessage('empty', COPY.empty);
        nextCursor = '';
        hideLoadMore();
        return;
      }
      clearStateMessage();
      renderResults(items);
      nextCursor = cursor;
      if (nextCursor) {
        showLoadMore();
      } else {
        hideLoadMore();
      }
    }).catch(function () {
      clearResults();
      setStateMessage('error', COPY.error);
      nextCursor = '';
      hideLoadMore();
    }).then(function () {
      setButtonDisabled(IDS.buttonSearch, false);
      setButtonDisabled(IDS.buttonMore, false);
    });
  }

  // loadMore (D30k) fetches the next cursor-paginated page of the most
  // recent first-page search and appends the results.
  //
  // Reuses the URLSearchParams snapshot captured by run(); never
  // re-reads live form values. The cursor is treated as opaque —
  // copied byte-for-byte into the outgoing query, never decoded or
  // inspected. URLSearchParams handles wire-level escaping.
  //
  // Behaviour:
  //
  //   - Existing results are preserved on success and on error.
  //   - The Load more button is hidden when the response omits
  //     next_cursor; the operator's pagination loop ends.
  //   - On error, the button stays visible (the operator can retry)
  //     and the error copy appears in the load-more state slot.
  //
  // Returns a Promise so tests can await it.
  function loadMore() {
    if (!currentQueryParams || !nextCursor) {
      // Defensive: the button should be hidden in this case; the
      // guard exists so a stale event handler cannot send a
      // cursor-less follow-up that would mimic a fresh first-page
      // search.
      return Promise.resolve();
    }
    var p = new URLSearchParams(currentQueryParams.toString());
    p.set('cursor', nextCursor);
    var url = ENDPOINT + '?' + p.toString();

    setLoadMoreState('loading', COPY.loadingMore);
    setButtonDisabled(IDS.buttonMore, true);
    setButtonDisabled(IDS.buttonSearch, true);

    return fetch(url, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json' },
    }).then(function (resp) {
      if (!resp.ok) {
        throw new Error('non-2xx status ' + resp.status);
      }
      return resp.json();
    }).then(function (body) {
      var items = (body && Array.isArray(body.items)) ? body.items : [];
      var cursor = (body && typeof body.next_cursor === 'string') ? body.next_cursor : '';
      if (items.length > 0) {
        appendResults(items);
      }
      // Accept the server's next_cursor as the source of truth: an
      // empty / absent value ends the pagination loop even if the
      // page itself was non-empty.
      nextCursor = cursor;
      if (nextCursor) {
        showLoadMore();
        clearLoadMoreState();
      } else {
        hideLoadMore();
      }
    }).catch(function () {
      // Preserve existing results. Surface the load-more-specific
      // error copy. Keep the button visible only if a cursor is
      // still available so the operator can retry.
      setLoadMoreState('error', COPY.errorMore);
      if (nextCursor) {
        showLoadMore();
      } else {
        hideLoadMore();
      }
    }).then(function () {
      setButtonDisabled(IDS.buttonMore, false);
      setButtonDisabled(IDS.buttonSearch, false);
    });
  }

  // clear resets every supported filter to its initial empty value and
  // restores the initial state copy. D30k additions: drop the cursor
  // accumulator and the captured first-page query params so a
  // subsequent fresh search starts from a clean state, and hide the
  // Load more affordance. No network call.
  function clear() {
    setValue(IDS.fieldEventType,     '');
    setValue(IDS.fieldEventTypes,    '');
    setValue(IDS.fieldEnvelopeID,    '');
    setValue(IDS.fieldRequestSource, '');
    setValue(IDS.fieldRequestID,     '');
    setValue(IDS.fieldSince,         '');
    setValue(IDS.fieldUntil,         '');
    setValue(IDS.fieldLimit,         '');
    // order has a default; reset it to the documented default 'desc'.
    setValue(IDS.fieldOrder,         'desc');
    clearResults();
    nextCursor = '';
    currentQueryParams = null;
    hideLoadMore();
    setStateMessage('initial', COPY.initial);
  }

  // -------------------------------------------------------------------------
  // Wiring.
  // -------------------------------------------------------------------------

  // init wires the form submit + Clear button + Load more button to
  // run/clear/loadMore. Safe to call multiple times — listeners are
  // idempotent because the form element is replaced on every page
  // load.
  function init() {
    var form = document.getElementById(IDS.form);
    if (form && !form.dataset.runtimeEvidenceWired) {
      form.addEventListener('submit', function (ev) {
        ev.preventDefault();
        run();
      });
      form.dataset.runtimeEvidenceWired = 'true';
    }
    var clearBtn = document.getElementById(IDS.buttonClear);
    if (clearBtn && !clearBtn.dataset.runtimeEvidenceWired) {
      clearBtn.addEventListener('click', function (ev) {
        ev.preventDefault();
        clear();
      });
      clearBtn.dataset.runtimeEvidenceWired = 'true';
    }
    var moreBtn = document.getElementById(IDS.buttonMore);
    if (moreBtn && !moreBtn.dataset.runtimeEvidenceWired) {
      moreBtn.addEventListener('click', function (ev) {
        ev.preventDefault();
        loadMore();
      });
      moreBtn.dataset.runtimeEvidenceWired = 'true';
    }
    var stateEl = document.getElementById(IDS.state);
    if (stateEl && !stateEl.dataset.runtimeEvidenceInitialised) {
      setStateMessage('initial', COPY.initial);
      stateEl.dataset.runtimeEvidenceInitialised = 'true';
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // -------------------------------------------------------------------------
  // Public namespace.
  // -------------------------------------------------------------------------

  window.MIDASExplorerRecords.evidenceSearch = {
    // Action surface — tests and the form wiring call these.
    run:      run,
    clear:    clear,
    init:     init,
    loadMore: loadMore,
    // Pure helpers exposed for tests.
    splitEventTypes:   splitEventTypes,
    buildQuery:        buildQuery,
    collectFormValues: collectFormValues,
    // Constants exposed so the markup pins and the JS stay aligned.
    IDS:  IDS,
    COPY: COPY,
    ENDPOINT: ENDPOINT,
  };
})();
