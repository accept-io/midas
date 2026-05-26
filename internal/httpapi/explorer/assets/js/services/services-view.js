// /explorer/assets/js/services/services-view.js — D32a-impl-5
//
// Production owner of the Explorer Services view orchestration. The
// inline IIFE previously owned ~530 lines of services code:
//   loadBusinessServicesList, renderServicesCatalogue,
//   setServicesSubView, showServicesCatalogue, showBusinessServiceRecord,
//   showBusinessServiceMap, renderServicesView, loadBusinessServiceRecord,
//   renderRecordFieldGrid, renderRecordSection, renderRelatedList,
//   renderBusinessServiceRecord, updateServicesCoverageSummary,
//   showServicesDriftOverview.
//
// D32a-impl-5 extracts the implementations here; the inline IIFE
// keeps thin shims so the existing call-sites in the rest of the
// inline code (handleGovernanceMapAction → showBusinessServiceRecord,
// wireServicesSubViewControls, etc.) continue to resolve.
//
// External dependencies (all already module-namespaced):
//   window.MIDASExplorerAPI.businessServices.list (D32a-impl-1)
//   window.MIDASExplorerGraph.shell.refresh (D32a-impl-2)
//   window.MIDASExplorerState.serviceRecordCache (D27j-ui-foundation-4)
//   window.MIDASExplorerUtils.escHtml / formatExternalRef /
//     formatFieldValue
//   window.MIDASExplorerStore (D32a-impl-1 — for selectedBusinessServiceId)
//
// Public surface on window.MIDASExplorerServices:
//
//   init(options)
//     Wires the sub-view control event handlers (search input,
//     back buttons, open-map button). Idempotent; called once at
//     load time. options.hooks bundles the still-inline orchestration
//     hooks (resetGraphState, gmapModeSet, refreshGovernanceMap,
//     updateBackButton, gmapHistoryClear).
//
//   loadCatalogue()
//     Fetches the business-service list via the API client and
//     renders the catalogue.
//
//   showCatalogue() / showRecord(id) / showMap(id) / showDriftOverview()
//     Sub-view entry points.
//
//   loadRecord(id) / renderRecord(payload) / renderCatalogue(filter)
//     Direct render entry points used by sub-view transitions.
//
//   setSubView(view) / getSubView()
//     Active sub-view: 'catalogue' / 'detail' / 'map' / 'drift'.
//
//   getSelectedServiceId() / setSelectedServiceId(id)
//     Selected business-service id; mirrored to ExplorerStore.

(function () {
  'use strict';

  function _api()      { return (window.MIDASExplorerAPI && window.MIDASExplorerAPI.businessServices) || null; }
  function _graph()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.shell) || null; }
  function _utils()    { return window.MIDASExplorerUtils || {}; }
  function _state()    { return window.MIDASExplorerState || {}; }
  function _store()    { return window.MIDASExplorerStore || null; }
  function _escHtml(s) { var fn = _utils().escHtml; return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s); }
  function _formatExternalRef(r) { var fn = _utils().formatExternalRef; return typeof fn === 'function' ? fn(r) : String(r || ''); }
  function _formatFieldValue(v)  { var fn = _utils().formatFieldValue;  return typeof fn === 'function' ? fn(v) : _escHtml(v == null ? '—' : String(v)); }

  // serviceRecordCache is an object map shared with the inline IIFE
  // through window.MIDASExplorerState (object identity preserved).
  function _cache() {
    var st = _state();
    st.serviceRecordCache = st.serviceRecordCache || {};
    return st.serviceRecordCache;
  }

  // Module-private state. The catalogue list / loading / error
  // primitives never need to live in the store (the inline IIFE's
  // old locals were also module-scoped). selectedBusinessServiceId
  // is mirrored to the store for shell consumers.
  var _selectedId = null;
  var _liveList   = null;
  var _liveError  = null;
  var _liveLoading = false;
  var _subView    = 'catalogue';
  var _recordLoading = false;
  var _recordError   = null;
  // Hook bundle from the inline IIFE (graph state reset, mode set,
  // map refresh, back button update). Wired by init().
  var _hooks = {};

  function _writeSelectedToStore(id) {
    var s = _store();
    if (s && typeof s.setState === 'function') {
      s.setState({ selectedBusinessServiceId: id || '' });
    }
  }

  function getSelectedServiceId() { return _selectedId; }
  function setSelectedServiceId(id) {
    _selectedId = id || null;
    _writeSelectedToStore(_selectedId);
  }

  function getSubView() { return _subView; }
  function setSubView(view) {
    _subView = (view === 'detail' || view === 'map' || view === 'drift') ? view : 'catalogue';
    var cat   = document.getElementById('services-catalogue-view');
    var rec   = document.getElementById('services-record-view');
    var map   = document.getElementById('services-map-view');
    var drift = document.getElementById('services-drift-view');
    if (cat)   cat.classList.toggle('active',   _subView === 'catalogue');
    if (rec)   rec.classList.toggle('active',   _subView === 'detail');
    if (map)   map.classList.toggle('active',   _subView === 'map');
    if (drift) drift.classList.toggle('active', _subView === 'drift');
    document.body.classList.toggle('gmap-mode', _subView === 'map');
  }

  function showDriftOverview() {
    setSubView('drift');
    if (typeof _hooks.setGmapMode === 'function') _hooks.setGmapMode('overview');
    if (window.MIDASExplorerDrift && typeof window.MIDASExplorerDrift.loadDriftHeatmap === 'function') {
      window.MIDASExplorerDrift.loadDriftHeatmap();
    }
  }

  function showCatalogue() {
    setSubView('catalogue');
    if (typeof _hooks.setGmapMode === 'function') _hooks.setGmapMode('overview');
    renderCatalogue();
  }

  function showRecord(serviceId) {
    if (!serviceId) { showCatalogue(); return; }
    setSelectedServiceId(serviceId);
    if (typeof _hooks.resetGraphState === 'function') _hooks.resetGraphState('service', serviceId);
    if (typeof _hooks.gmapHistoryClear === 'function') _hooks.gmapHistoryClear();
    if (typeof _hooks.updateBackButton === 'function') _hooks.updateBackButton();
    setSubView('detail');
    if (typeof _hooks.setGmapMode === 'function') _hooks.setGmapMode('overview');
    loadRecord(serviceId);
  }

  function showMap(serviceId) {
    if (!serviceId) { showCatalogue(); return; }
    setSelectedServiceId(serviceId);
    if (typeof _hooks.resetGraphState === 'function') _hooks.resetGraphState('service', serviceId);
    if (typeof _hooks.gmapHistoryClear === 'function') _hooks.gmapHistoryClear();
    if (typeof _hooks.updateBackButton === 'function') _hooks.updateBackButton();
    setSubView('map');
    var cached = _cache()[serviceId];
    var label  = document.getElementById('services-map-back-label');
    var bsName = (cached && cached.business_service && (cached.business_service.name || cached.business_service.id)) || serviceId;
    if (label) label.textContent = bsName;
    if (typeof _hooks.setGmapMode === 'function') _hooks.setGmapMode('map');
    if (typeof _hooks.resetGmapLastBSId === 'function') _hooks.resetGmapLastBSId();
    if (typeof _hooks.refreshGovernanceMap === 'function') _hooks.refreshGovernanceMap();
  }

  function loadCatalogue() {
    if (_liveLoading) return;
    _liveLoading = true;
    _liveError = null;
    renderCatalogue();
    var api = _api();
    if (!api || typeof api.list !== 'function') {
      _liveLoading = false;
      _liveError = { status: 0, message: 'API client not available' };
      _liveList = null;
      renderCatalogue();
      return;
    }
    api.list().then(function (payload) {
      _liveLoading = false;
      if (payload && payload.__status) {
        _liveError = { status: payload.__status, message: 'HTTP ' + payload.__status };
        _liveList = null;
        renderCatalogue();
        return;
      }
      _liveList = (payload && payload.business_services) || [];
      _liveError = null;
      if (_liveList.length === 0) {
        setSelectedServiceId(null);
      } else if (!_liveList.some(function (bs) { return bs.id === _selectedId; })) {
        setSelectedServiceId(_liveList[0].id);
      }
      renderView();
    }).catch(function (err) {
      _liveLoading = false;
      _liveError = { status: 0, message: (err && err.message) || 'fetch failed' };
      _liveList = null;
      renderCatalogue();
    });
  }

  function renderView() {
    if (_subView === 'detail') {
      renderRecord(_selectedId && _cache()[_selectedId]);
    } else if (_subView === 'map') {
      // Map sub-view drives its own state; leave alone.
    } else {
      renderCatalogue();
    }
  }

  function renderCatalogue(filter) {
    var list = document.getElementById('services-bs-list');
    if (!list) return;
    var q = (filter || '').toLowerCase().trim();

    if (_liveLoading) {
      list.innerHTML = '<div class="services-bs-loading">Loading business services…</div>';
      return;
    }
    if (_liveError) {
      var status = _liveError.status ? ('HTTP ' + _liveError.status) : 'Network error';
      list.innerHTML =
        '<div class="services-bs-error">' +
          '<div><strong>Could not load business services</strong></div>' +
          '<div>' + _escHtml(status) + (_liveError.message ? ' — ' + _escHtml(_liveError.message) : '') + '</div>' +
        '</div>';
      return;
    }
    if (!Array.isArray(_liveList)) {
      list.innerHTML = '<div class="services-bs-loading">Loading business services…</div>';
      return;
    }
    if (_liveList.length === 0) {
      list.innerHTML =
        '<div class="services-bs-empty">' +
          '<div><strong>No business services found</strong></div>' +
          '<div>Apply a BusinessService bundle to populate this list.</div>' +
        '</div>';
      return;
    }

    var filtered = _liveList.filter(function (bs) {
      return !q ||
        (bs.name || '').toLowerCase().indexOf(q) >= 0 ||
        (bs.id || '').toLowerCase().indexOf(q) >= 0;
    });
    if (filtered.length === 0) {
      var empty = document.createElement('div');
      empty.className = 'scenario-no-results';
      empty.textContent = 'No services match “' + q + '”';
      list.replaceChildren(empty);
      return;
    }

    list.innerHTML = filtered.map(function (bs) {
      var id = bs.id;
      var status   = bs.status ? '<span class="services-bs-card-status">' + _escHtml(bs.status) + '</span>' : '';
      var typePill = bs.service_type ? '<span class="services-bs-card-type">' + _escHtml(bs.service_type) + '</span>' : '';
      var ownerPill= bs.owner_id ? '<span class="services-bs-card-owner">owner: ' + _escHtml(bs.owner_id) + '</span>' : '';
      var extPill  = bs.external_ref ? '<span class="services-bs-card-extref">EXT-REF</span>' : '';
      var meta = (typePill || ownerPill || extPill)
        ? '<div class="services-bs-card-meta">' + typePill + ownerPill + extPill + '</div>'
        : '';
      return (
        '<button type="button" class="services-bs-card services-catalogue-card inactive" data-service-id="' + _escHtml(id) + '">' +
          '<div class="services-bs-card-header">' +
            '<span class="services-bs-card-id">' + _escHtml(id) + '</span>' +
            status +
          '</div>' +
          '<div class="services-bs-card-name">' + _escHtml(bs.name || id) + '</div>' +
          meta +
        '</button>'
      );
    }).join('');

    var cards = list.querySelectorAll('.services-bs-card');
    cards.forEach(function (btn) {
      btn.addEventListener('click', function () {
        showRecord(btn.dataset.serviceId);
      });
    });
  }

  // ── Record page ──────────────────────────────────────────────────────────

  function loadRecord(serviceId) {
    var cache = _cache();
    var cached = cache[serviceId];
    if (cached) {
      _recordLoading = false;
      _recordError = null;
      renderRecord(cached);
      return;
    }
    _recordLoading = true;
    _recordError = null;
    renderRecord(null);
    if (typeof _hooks.resetGraphState === 'function') _hooks.resetGraphState('service', serviceId);
    if (typeof _hooks.gmapHistoryClear === 'function') _hooks.gmapHistoryClear();
    if (typeof _hooks.updateBackButton === 'function') _hooks.updateBackButton();
    var graph = _graph();
    if (!graph || typeof graph.refresh !== 'function') {
      _recordLoading = false;
      _recordError = { status: 0, message: 'graph shell not available' };
      renderRecord(null);
      return;
    }
    graph.refresh({ lens: 'context', view: 'service', id: serviceId, depth: 5 })
      .then(function (payloadOrLayout) {
        _recordLoading = false;
        if (payloadOrLayout && payloadOrLayout.__status) {
          _recordError = { status: payloadOrLayout.__status, message: 'HTTP ' + payloadOrLayout.__status };
          renderRecord(null);
          return;
        }
        if (serviceId !== _selectedId) return;
        cache[serviceId] = payloadOrLayout;
        _recordError = null;
        renderRecord(payloadOrLayout);
      })
      .catch(function (err) {
        _recordLoading = false;
        _recordError = { status: 0, message: (err && err.message) || 'fetch failed' };
        renderRecord(null);
      });
  }

  function _renderFieldGrid(rows) {
    if (!rows || rows.length === 0) return '';
    var cells = rows.map(function (pair) {
      var k = pair[0], v = pair[1];
      return '<div class="services-record-field-key">' + _escHtml(String(k)) + '</div>' + _formatFieldValue(v);
    }).join('');
    return '<div class="services-record-field-grid">' + cells + '</div>';
  }

  function _renderSection(title, contentHtml) {
    if (!contentHtml) return '';
    return (
      '<section class="services-record-section">' +
        '<div class="services-record-section-title">' + _escHtml(title) + '</div>' +
        contentHtml +
      '</section>'
    );
  }

  function _renderRelatedList(rows, emptyMessage) {
    if (!rows || rows.length === 0) {
      return '<div class="services-record-empty">' + _escHtml(emptyMessage) + '</div>';
    }
    return '<div class="services-related-list">' +
      rows.map(function (r) {
        return '<div class="services-related-row">' +
          (r.id   ? '<span class="id">' + _escHtml(r.id) + '</span>' : '') +
          (r.name ? '<span class="name">' + _escHtml(r.name) + '</span>' : '') +
          (r.meta ? '<span class="meta">' + _escHtml(r.meta) + '</span>' : '') +
          (r.badge ? '<span class="badge ' + _escHtml(r.badgeCls || '') + '">' + _escHtml(r.badge) + '</span>' : '') +
        '</div>';
      }).join('') +
    '</div>';
  }

  function renderRecord(payload) {
    var body     = document.getElementById('services-record-body');
    var nameEl   = document.getElementById('services-record-name');
    var idEl     = document.getElementById('services-record-id');
    var statusEl = document.getElementById('services-record-status');
    if (!body || !nameEl || !idEl || !statusEl) return;

    if (_recordLoading) {
      body.innerHTML = '<div class="services-record-loading">Loading record…</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }
    if (_recordError) {
      var status = _recordError.status ? ('HTTP ' + _recordError.status) : 'Network error';
      body.innerHTML =
        '<div class="services-record-error">' +
          '<div><strong>Could not load record</strong></div>' +
          '<div>' + _escHtml(status) + (_recordError.message ? ' — ' + _escHtml(_recordError.message) : '') + '</div>' +
        '</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }
    if (!payload || !payload.business_service) {
      body.innerHTML = '<div class="services-record-empty">No record loaded.</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }

    var bs = payload.business_service;
    nameEl.textContent = bs.name || bs.id;
    idEl.textContent = bs.id;
    statusEl.textContent = bs.status || '';

    var sections = [];

    // 1. Identity strip.
    var identityPills = [];
    if (bs.status)       identityPills.push({ key: 'status', val: bs.status });
    if (bs.service_type) identityPills.push({ key: 'type',   val: bs.service_type });
    if (bs.owner_id)     identityPills.push({ key: 'owner',  val: bs.owner_id });
    if (bs.external_ref) identityPills.push({ key: 'ext',    val: 'EXT-REF' });
    if (identityPills.length) {
      var pills = identityPills.map(function (p) {
        return '<span class="services-record-identity-pill"><span class="key">' + _escHtml(p.key) + '</span>' + _escHtml(p.val) + '</span>';
      }).join('');
      sections.push(_renderSection('Identity', '<div class="services-record-identity">' + pills + '</div>'));
    }

    // 2. Core details.
    var coreRows = [
      ['id',               bs.id],
      ['name',             bs.name],
      ['description',      bs.description],
      ['service_type',     bs.service_type],
      ['owner_id',         bs.owner_id],
      ['regulatory_scope', bs.regulatory_scope],
      ['external_ref',     bs.external_ref ? _formatExternalRef(bs.external_ref) : ''],
    ];
    sections.push(_renderSection('Core details', _renderFieldGrid(coreRows)));

    // 3. Related services.
    var rels   = (payload.relationships && payload.relationships.outgoing) || [];
    var relsIn = (payload.relationships && payload.relationships.incoming) || [];
    var relRows = [];
    rels.forEach(function (r)   { relRows.push({ id: r.id, name: '→ ' + (r.other_name || r.target_business_service_id || ''), meta: r.relationship_type || '' }); });
    relsIn.forEach(function (r) { relRows.push({ id: r.id, name: '← ' + (r.other_name || r.source_business_service_id || ''), meta: r.relationship_type || '' }); });
    sections.push(_renderSection('Related services', _renderRelatedList(relRows, 'No related services')));

    // 4. Capabilities.
    var caps = (payload.capabilities || []).map(function (c) {
      return { id: c.id, name: c.name || c.id, meta: c.status || '' };
    });
    sections.push(_renderSection('Capabilities', _renderRelatedList(caps, 'No capabilities linked')));

    // 5. Processes.
    var procs = (payload.processes || []).map(function (p) {
      return { id: p.id, name: p.name || p.id, meta: p.status || '' };
    });
    sections.push(_renderSection('Processes', _renderRelatedList(procs, 'No processes linked')));

    // 6. Decision surfaces.
    var surfs = (payload.surfaces || []).map(function (s) {
      var hasBinding = (s.ai_bindings || []).length > 0;
      var counts = [];
      if (s.profile_count != null) counts.push('p=' + s.profile_count);
      if (s.grant_count != null)   counts.push('g=' + s.grant_count);
      if (s.agent_count != null)   counts.push('a=' + s.agent_count);
      return {
        id: s.id + (s.version != null ? '·v' + s.version : ''),
        name: s.name || s.id,
        meta: [s.status || '', s.process_id ? 'proc=' + s.process_id : '', counts.join(' ')].filter(Boolean).join(' · '),
        badge: hasBinding ? 'AI bound' : 'no AI',
        badgeCls: hasBinding ? 'bind' : 'warn',
      };
    });
    sections.push(_renderSection('Decision surfaces', _renderRelatedList(surfs, 'No decision surfaces under this service')));

    // 7. AI systems.
    var ais = (payload.ai_systems || []).map(function (ai) {
      var ver = ai.active_version && ai.active_version.version != null ? 'v' + ai.active_version.version : '';
      var bcount = (ai.bindings || []).length;
      var meta = [ai.vendor, ai.system_type, ai.status, ver, bcount + ' binding' + (bcount === 1 ? '' : 's')]
        .filter(Boolean).join(' · ');
      return { id: ai.id, name: ai.name || ai.id, meta: meta };
    });
    sections.push(_renderSection('AI systems', _renderRelatedList(ais, 'No AI systems linked')));

    // 8. Governance summary.
    var auth = payload.authority_summary || {};
    var cov  = payload.coverage || {};
    var chips = [
      { label: 'Surfaces',        value: auth.surface_count || 0 },
      { label: 'Active profiles', value: auth.active_profile_count || 0 },
      { label: 'Active grants',   value: auth.active_grant_count || 0 },
      { label: 'Active agents',   value: auth.active_agent_count || 0 },
      { label: 'AI-bound',        value: cov.surfaces_with_direct_ai_binding || 0 },
      { label: 'Coverage gaps',   value: cov.surfaces_with_no_ai_binding || 0, gap: (cov.surfaces_with_no_ai_binding || 0) > 0 },
    ];
    var chipHtml = '<div class="services-record-chips">' +
      chips.map(function (c) {
        return '<div class="services-record-chip' + (c.gap ? ' gap' : '') + '">' +
          '<div class="label">' + _escHtml(c.label) + '</div>' +
          '<div class="value">' + _escHtml(String(c.value)) + '</div>' +
        '</div>';
      }).join('') +
    '</div>';
    sections.push(_renderSection('Governance summary', chipHtml));

    body.innerHTML = sections.filter(Boolean).join('');
  }

  // Compatibility helpers exposed for the inline IIFE — these are
  // pure HTML composers reused by the capability record renderer.
  function renderRecordFieldGrid(rows) { return _renderFieldGrid(rows); }
  function renderRecordSection(title, html) { return _renderSection(title, html); }
  function renderRelatedList(rows, empty) { return _renderRelatedList(rows, empty); }

  // updateCoverageSummary is retained as a null-safe no-op for
  // compatibility with refreshCoverage()'s hook.
  function updateCoverageSummary(records) {
    var pctEl  = document.getElementById('services-coverage-pct');
    var barEl  = document.getElementById('services-coverage-bar-fill');
    var noteEl = document.getElementById('services-coverage-note');
    if (!pctEl || !barEl || !noteEl) return;
    var count = Array.isArray(records) ? records.length : 0;
    pctEl.textContent = String(count);
    barEl.style.width = count > 0 ? '100%' : '0%';
    noteEl.textContent = count === 0
      ? 'No coverage records yet — run an evaluation to populate.'
      : 'Records visible in the Records view.';
  }

  function init(options) {
    options = options || {};
    if (options.hooks && typeof options.hooks === 'object') {
      _hooks = options.hooks;
    }
    var search = document.getElementById('services-bs-search');
    if (search) search.addEventListener('input', function (e) { renderCatalogue(e.target.value); });
    var backToCat = document.getElementById('services-record-back-btn');
    if (backToCat) backToCat.addEventListener('click', showCatalogue);
    var openMap = document.getElementById('services-record-open-map-btn');
    if (openMap) openMap.addEventListener('click', function () { showMap(_selectedId); });
    var backToRec = document.getElementById('services-map-back-btn');
    if (backToRec) backToRec.addEventListener('click', function () { showRecord(_selectedId); });
    var openDrift = document.getElementById('services-drift-open-btn');
    if (openDrift) openDrift.addEventListener('click', showDriftOverview);
    var backFromDrift = document.getElementById('services-drift-back-btn');
    if (backFromDrift) backFromDrift.addEventListener('click', showCatalogue);
  }

  window.MIDASExplorerServices = {
    init:                     init,
    loadCatalogue:            loadCatalogue,
    showCatalogue:            showCatalogue,
    showRecord:               showRecord,
    showMap:                  showMap,
    showDriftOverview:        showDriftOverview,
    renderView:               renderView,
    renderCatalogue:          renderCatalogue,
    loadRecord:               loadRecord,
    renderRecord:             renderRecord,
    setSubView:               setSubView,
    getSubView:               getSubView,
    getSelectedServiceId:     getSelectedServiceId,
    setSelectedServiceId:     setSelectedServiceId,
    // Helpers shared with the capability view module.
    renderRecordFieldGrid:    renderRecordFieldGrid,
    renderRecordSection:      renderRecordSection,
    renderRelatedList:        renderRelatedList,
    updateCoverageSummary:    updateCoverageSummary,
  };
})();
