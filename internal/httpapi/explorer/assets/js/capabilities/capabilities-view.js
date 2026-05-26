// /explorer/assets/js/capabilities/capabilities-view.js — D32a-impl-5
//
// Production owner of the Explorer Capabilities view orchestration.
// The inline IIFE previously owned ~580 lines of capabilities code:
//   loadCapabilitiesList, renderCapabilitiesCatalogue,
//   setCapabilitiesSubView, showCapabilitiesCatalogue,
//   showCapabilityRecord, loadCapabilityRecord, loadCapabilityChildren,
//   loadCapabilityBusinessServices, loadCapabilityAIBindings,
//   renderCapabilityRecord, renderCapabilityChildrenSection,
//   renderCapabilityBusinessServicesSection,
//   renderCapabilityAIBindingsSection.
//
// D32a-impl-5 extracts the implementations here; the inline IIFE
// keeps thin shims so the existing call-sites
// (handleGovernanceMapAction → showCapabilityRecord, sidebar nav,
// wireCapabilitiesSubViewControls) continue to resolve.
//
// External dependencies:
//   window.MIDASExplorerAPI.capabilities.list / get / children /
//     businessServices / aiBindings
//   window.MIDASExplorerState.capabilityRecordCache (D27j-ui-foundation-4)
//   window.MIDASExplorerServices.renderRelatedList /
//     showRecord (for cross-link)
//   window.MIDASExplorerUtils.escHtml / formatExternalRef
//   window.MIDASExplorerStore (D32a-impl-1)
//
// Public surface on window.MIDASExplorerCapabilities:
//   init(options)
//   loadCatalogue()
//   showCatalogue() / showRecord(id)
//   renderCatalogue(filter) / renderRecord(payload)
//   loadRecord(id)
//   setSubView(view) / getSubView()
//   getSelectedCapabilityId() / setSelectedCapabilityId(id)

(function () {
  'use strict';

  function _api()      { return (window.MIDASExplorerAPI && window.MIDASExplorerAPI.capabilities) || null; }
  function _services() { return window.MIDASExplorerServices || null; }
  function _utils()    { return window.MIDASExplorerUtils || {}; }
  function _state()    { return window.MIDASExplorerState || {}; }
  function _store()    { return window.MIDASExplorerStore || null; }
  function _escHtml(s) { var fn = _utils().escHtml; return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s); }
  function _formatExternalRef(r) { var fn = _utils().formatExternalRef; return typeof fn === 'function' ? fn(r) : String(r || ''); }

  function _cache() {
    var st = _state();
    st.capabilityRecordCache = st.capabilityRecordCache || {};
    return st.capabilityRecordCache;
  }

  function _renderRelatedList(rows, empty) {
    var svc = _services();
    if (svc && typeof svc.renderRelatedList === 'function') return svc.renderRelatedList(rows, empty);
    if (!rows || rows.length === 0) return '<div class="services-record-empty">' + _escHtml(empty) + '</div>';
    return '<div class="services-related-list">' + rows.map(function (r) {
      return '<div class="services-related-row">' +
        (r.id   ? '<span class="id">'   + _escHtml(r.id)   + '</span>' : '') +
        (r.name ? '<span class="name">' + _escHtml(r.name) + '</span>' : '') +
        (r.meta ? '<span class="meta">' + _escHtml(r.meta) + '</span>' : '') +
      '</div>';
    }).join('') + '</div>';
  }

  // Module-private state. Mirrors the inline IIFE's old locals.
  var _selectedId    = null;
  var _liveList      = null;
  var _liveError     = null;
  var _liveLoading   = false;
  var _subView       = 'catalogue';
  var _recordLoading = false;
  var _recordError   = null;
  // Per-capability sub-resource state (children / business-services / ai-bindings).
  var _childrenCache = {};
  var _childrenLoading = {};
  var _childrenError = {};
  var _bsCache = {};
  var _bsLoading = {};
  var _bsError = {};
  var _aiCache = {};
  var _aiLoading = {};
  var _aiError = {};

  function _writeSelectedToStore(id) {
    var s = _store();
    if (s && typeof s.setState === 'function') {
      s.setState({ selectedCapabilityId: id || '' });
    }
  }

  function getSelectedCapabilityId() { return _selectedId; }
  function setSelectedCapabilityId(id) {
    _selectedId = id || null;
    _writeSelectedToStore(_selectedId);
  }
  function getSubView() { return _subView; }
  function setSubView(view) {
    _subView = view === 'detail' ? 'detail' : 'catalogue';
    var cat = document.getElementById('capabilities-catalogue-view');
    var rec = document.getElementById('capabilities-record-view');
    if (cat) cat.classList.toggle('active', _subView === 'catalogue');
    if (rec) rec.classList.toggle('active', _subView === 'detail');
  }

  function showCatalogue() {
    setSubView('catalogue');
    renderCatalogue();
  }

  function showRecord(capId) {
    if (!capId) { showCatalogue(); return; }
    setSelectedCapabilityId(capId);
    setSubView('detail');
    loadRecord(capId);
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
      _liveList = Array.isArray(payload) ? payload : [];
      _liveError = null;
      renderCatalogue();
    }).catch(function (err) {
      _liveLoading = false;
      _liveError = { status: 0, message: (err && err.message) || 'fetch failed' };
      _liveList = null;
      renderCatalogue();
    });
  }

  function renderCatalogue(filter) {
    var list = document.getElementById('capabilities-list');
    if (!list) return;
    var q = (filter || '').toLowerCase().trim();

    if (_liveLoading) {
      list.innerHTML = '<div class="capabilities-loading">Loading capabilities…</div>';
      return;
    }
    if (_liveError) {
      var status = _liveError.status ? ('HTTP ' + _liveError.status) : 'Network error';
      list.innerHTML =
        '<div class="capabilities-error">' +
          '<div><strong>Could not load capabilities</strong></div>' +
          '<div>' + _escHtml(status) + (_liveError.message ? ' — ' + _escHtml(_liveError.message) : '') + '</div>' +
        '</div>';
      return;
    }
    if (!Array.isArray(_liveList)) {
      list.innerHTML = '<div class="capabilities-loading">Loading capabilities…</div>';
      return;
    }
    if (_liveList.length === 0) {
      list.innerHTML =
        '<div class="capabilities-empty">' +
          '<div><strong>No capabilities found</strong></div>' +
          '<div>Apply a Capability bundle to populate this list.</div>' +
        '</div>';
      return;
    }

    var filtered = _liveList.filter(function (c) {
      return !q ||
        (c.name || '').toLowerCase().indexOf(q) >= 0 ||
        (c.id || '').toLowerCase().indexOf(q) >= 0 ||
        (c.owner || '').toLowerCase().indexOf(q) >= 0 ||
        (c.description || '').toLowerCase().indexOf(q) >= 0;
    });
    if (filtered.length === 0) {
      var empty = document.createElement('div');
      empty.className = 'capabilities-no-match';
      empty.textContent = 'No capabilities match “' + q + '”';
      list.replaceChildren(empty);
      return;
    }

    list.innerHTML = filtered.map(function (c) {
      var id = c.id;
      var status = c.status ? '<span class="capabilities-card-status">' + _escHtml(c.status) + '</span>' : '';
      var desc = c.description ? '<div class="capabilities-card-desc">' + _escHtml(c.description) + '</div>' : '';
      var ownerPill = c.owner ? '<span class="capabilities-card-owner">owner: ' + _escHtml(c.owner) + '</span>' : '';
      var meta = ownerPill ? '<div class="capabilities-card-meta">' + ownerPill + '</div>' : '';
      return (
        '<button type="button" class="capabilities-card" data-capability-id="' + _escHtml(id) + '">' +
          '<div class="capabilities-card-header">' +
            '<span class="capabilities-card-id">' + _escHtml(id) + '</span>' +
            status +
          '</div>' +
          '<div class="capabilities-card-name">' + _escHtml(c.name || id) + '</div>' +
          desc +
          meta +
        '</button>'
      );
    }).join('');

    list.querySelectorAll('.capabilities-card').forEach(function (btn) {
      btn.addEventListener('click', function () {
        showRecord(btn.dataset.capabilityId);
      });
    });
  }

  function loadRecord(capId) {
    var cache = _cache();
    var cached = cache[capId];
    if (cached) {
      _recordLoading = false;
      _recordError = null;
      renderRecord(cached);
      loadChildren(capId);
      loadBusinessServices(capId);
      loadAIBindings(capId);
      return;
    }
    _recordLoading = true;
    _recordError = null;
    renderRecord(null);
    var api = _api();
    if (!api || typeof api.get !== 'function') {
      _recordLoading = false;
      _recordError = { status: 0, message: 'API client not available' };
      renderRecord(null);
      return;
    }
    api.get(capId).then(function (payload) {
      _recordLoading = false;
      if (payload && payload.__status) {
        _recordError = { status: payload.__status, message: 'HTTP ' + payload.__status };
        renderRecord(null);
        return;
      }
      if (capId !== _selectedId) return;
      cache[capId] = payload;
      _recordError = null;
      renderRecord(payload);
      loadChildren(capId);
      loadBusinessServices(capId);
      loadAIBindings(capId);
    }).catch(function (err) {
      _recordLoading = false;
      _recordError = { status: 0, message: (err && err.message) || 'fetch failed' };
      renderRecord(null);
    });
  }

  function loadChildren(capId) {
    if (_childrenCache[capId]) { renderChildrenSection(capId); return; }
    if (_childrenLoading[capId]) return;
    _childrenLoading[capId] = true;
    _childrenError[capId] = null;
    renderChildrenSection(capId);
    var api = _api();
    if (!api || typeof api.children !== 'function') {
      _childrenLoading[capId] = false;
      _childrenError[capId] = { status: 0, message: 'API client not available' };
      if (capId === _selectedId) renderChildrenSection(capId);
      return;
    }
    api.children(capId).then(function (payload) {
      _childrenLoading[capId] = false;
      if (payload && payload.__status) {
        _childrenError[capId] = { status: payload.__status, message: 'HTTP ' + payload.__status };
        if (capId === _selectedId) renderChildrenSection(capId);
        return;
      }
      _childrenCache[capId] = (payload && Array.isArray(payload.capabilities)) ? payload.capabilities : [];
      _childrenError[capId] = null;
      if (capId === _selectedId) renderChildrenSection(capId);
    }).catch(function (err) {
      _childrenLoading[capId] = false;
      _childrenError[capId] = { status: 0, message: (err && err.message) || 'fetch failed' };
      if (capId === _selectedId) renderChildrenSection(capId);
    });
  }

  function loadBusinessServices(capId) {
    if (_bsCache[capId]) { renderBusinessServicesSection(capId); return; }
    if (_bsLoading[capId]) return;
    _bsLoading[capId] = true;
    _bsError[capId] = null;
    renderBusinessServicesSection(capId);
    var api = _api();
    if (!api || typeof api.businessServices !== 'function') {
      _bsLoading[capId] = false;
      _bsError[capId] = { status: 0, message: 'API client not available' };
      if (capId === _selectedId) renderBusinessServicesSection(capId);
      return;
    }
    api.businessServices(capId).then(function (payload) {
      _bsLoading[capId] = false;
      if (payload && payload.__status) {
        _bsError[capId] = { status: payload.__status, message: 'HTTP ' + payload.__status };
        if (capId === _selectedId) renderBusinessServicesSection(capId);
        return;
      }
      _bsCache[capId] = (payload && Array.isArray(payload.business_services)) ? payload.business_services : [];
      _bsError[capId] = null;
      if (capId === _selectedId) renderBusinessServicesSection(capId);
    }).catch(function (err) {
      _bsLoading[capId] = false;
      _bsError[capId] = { status: 0, message: (err && err.message) || 'fetch failed' };
      if (capId === _selectedId) renderBusinessServicesSection(capId);
    });
  }

  function loadAIBindings(capId) {
    if (_aiCache[capId]) { renderAIBindingsSection(capId); return; }
    if (_aiLoading[capId]) return;
    _aiLoading[capId] = true;
    _aiError[capId] = null;
    renderAIBindingsSection(capId);
    var api = _api();
    if (!api || typeof api.aiBindings !== 'function') {
      _aiLoading[capId] = false;
      _aiError[capId] = { status: 0, message: 'API client not available' };
      if (capId === _selectedId) renderAIBindingsSection(capId);
      return;
    }
    api.aiBindings(capId).then(function (payload) {
      _aiLoading[capId] = false;
      if (payload && payload.__status) {
        _aiError[capId] = { status: payload.__status, message: 'HTTP ' + payload.__status };
        if (capId === _selectedId) renderAIBindingsSection(capId);
        return;
      }
      _aiCache[capId] = (payload && Array.isArray(payload.bindings)) ? payload.bindings : [];
      _aiError[capId] = null;
      if (capId === _selectedId) renderAIBindingsSection(capId);
    }).catch(function (err) {
      _aiLoading[capId] = false;
      _aiError[capId] = { status: 0, message: (err && err.message) || 'fetch failed' };
      if (capId === _selectedId) renderAIBindingsSection(capId);
    });
  }

  function renderRecord(payload) {
    var body     = document.getElementById('capabilities-record-body');
    var nameEl   = document.getElementById('capabilities-record-name');
    var idEl     = document.getElementById('capabilities-record-id');
    var statusEl = document.getElementById('capabilities-record-status');
    if (!body || !nameEl || !idEl || !statusEl) return;

    if (_recordLoading) {
      body.innerHTML = '<div class="capabilities-record-loading">Loading record…</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }
    if (_recordError) {
      var status = _recordError.status ? ('HTTP ' + _recordError.status) : 'Network error';
      body.innerHTML =
        '<div class="capabilities-record-error">' +
          '<div><strong>Could not load capability</strong></div>' +
          '<div>' + _escHtml(status) + (_recordError.message ? ' — ' + _escHtml(_recordError.message) : '') + '</div>' +
        '</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }
    if (!payload || typeof payload !== 'object') {
      body.innerHTML = '<div class="capabilities-record-empty">No record loaded.</div>';
      nameEl.textContent = _selectedId || '—';
      idEl.textContent = _selectedId || '';
      statusEl.textContent = '';
      return;
    }

    nameEl.textContent = payload.name || payload.id || '—';
    idEl.textContent = payload.id || '';
    statusEl.textContent = payload.status || '';

    var sections = [];

    // 1. Identity strip.
    var identityPills = [];
    if (payload.status)               identityPills.push({ key: 'status',   val: payload.status });
    if (payload.owner)                identityPills.push({ key: 'owner',    val: payload.owner });
    if (payload.parent_capability_id) identityPills.push({ key: 'parent',   val: payload.parent_capability_id });
    if (payload.origin)               identityPills.push({ key: 'origin',   val: payload.origin });
    if (payload.managed != null)      identityPills.push({ key: 'managed',  val: String(payload.managed) });
    if (payload.replaces)             identityPills.push({ key: 'replaces', val: payload.replaces });
    if (payload.external_ref)         identityPills.push({ key: 'ext',      val: 'EXT-REF' });
    if (identityPills.length) {
      var pills = identityPills.map(function (p) {
        return '<span class="capabilities-record-identity-pill"><span class="key">' + _escHtml(p.key) + '</span>' + _escHtml(p.val) + '</span>';
      }).join('');
      sections.push(
        '<section class="capabilities-record-section">' +
          '<div class="capabilities-record-section-title">Identity</div>' +
          '<div class="capabilities-record-identity">' + pills + '</div>' +
        '</section>'
      );
    }

    // 2. Core details.
    var managedVal = payload.managed == null ? '' : String(payload.managed);
    var cells = [
      ['id',                   payload.id],
      ['name',                 payload.name],
      ['description',          payload.description],
      ['status',               payload.status],
      ['owner',                payload.owner],
      ['parent_capability_id', payload.parent_capability_id],
      ['origin',               payload.origin],
      ['managed',              managedVal],
      ['replaces',             payload.replaces],
      ['created_by',           payload.created_by],
      ['created_at',           payload.created_at],
      ['updated_at',           payload.updated_at],
      ['external_ref',         payload.external_ref ? _formatExternalRef(payload.external_ref) : ''],
    ].map(function (pair) {
      var k = pair[0], v = pair[1];
      return '<div class="capabilities-record-field-key">' + _escHtml(k) + '</div>' +
        (v == null || v === ''
          ? '<span class="capabilities-record-field-val muted">—</span>'
          : '<span class="capabilities-record-field-val">' + _escHtml(String(v)) + '</span>');
    }).join('');
    sections.push(
      '<section class="capabilities-record-section">' +
        '<div class="capabilities-record-section-title">Core details</div>' +
        '<div class="capabilities-record-field-grid">' + cells + '</div>' +
      '</section>'
    );

    // 3. Child capabilities — populated by renderChildrenSection.
    sections.push(
      '<section class="capabilities-record-section">' +
        '<div class="capabilities-record-section-title">Child capabilities</div>' +
        '<div id="capabilities-record-children"></div>' +
      '</section>'
    );

    // 4. Business Services using this Capability.
    sections.push(
      '<section class="capabilities-record-section">' +
        '<div class="capabilities-record-section-title">Business Services using this Capability</div>' +
        '<div id="capabilities-record-business-services"></div>' +
      '</section>'
    );

    // 5. AI System bindings.
    sections.push(
      '<section class="capabilities-record-section">' +
        '<div class="capabilities-record-section-title">AI System bindings</div>' +
        '<div id="capabilities-record-ai-bindings"></div>' +
      '</section>'
    );

    body.innerHTML = sections.join('');

    var capId = payload.id || _selectedId;
    if (capId) {
      renderChildrenSection(capId);
      renderBusinessServicesSection(capId);
      renderAIBindingsSection(capId);
    }
  }

  function renderChildrenSection(capId) {
    var el = document.getElementById('capabilities-record-children');
    if (!el) return;
    if (_childrenLoading[capId]) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading child capabilities…</div>';
      return;
    }
    var err = _childrenError[capId];
    if (err) {
      var status = err.status ? ('HTTP ' + err.status) : 'Network error';
      el.innerHTML =
        '<div class="capabilities-record-error">' +
          '<div><strong>Could not load child capabilities</strong></div>' +
          '<div>' + _escHtml(status) + (err.message ? ' — ' + _escHtml(err.message) : '') + '</div>' +
        '</div>';
      return;
    }
    var children = _childrenCache[capId];
    if (!Array.isArray(children)) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading child capabilities…</div>';
      return;
    }
    var rows = children.map(function (c) { return { id: c.id, name: c.name || c.id, meta: c.status || '' }; });
    el.innerHTML = _renderRelatedList(rows, 'No child capabilities');
    el.querySelectorAll('.services-related-row').forEach(function (row, idx) {
      var child = children[idx];
      if (child && child.id) {
        row.style.cursor = 'pointer';
        row.addEventListener('click', function () { showRecord(child.id); });
      }
    });
  }

  function renderBusinessServicesSection(capId) {
    var el = document.getElementById('capabilities-record-business-services');
    if (!el) return;
    if (_bsLoading[capId]) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading business services…</div>';
      return;
    }
    var err = _bsError[capId];
    if (err) {
      var status = err.status ? ('HTTP ' + err.status) : 'Network error';
      el.innerHTML =
        '<div class="capabilities-record-error">' +
          '<div><strong>Could not load business services</strong></div>' +
          '<div>' + _escHtml(status) + (err.message ? ' — ' + _escHtml(err.message) : '') + '</div>' +
        '</div>';
      return;
    }
    var services = _bsCache[capId];
    if (!Array.isArray(services)) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading business services…</div>';
      return;
    }
    var rows = services.map(function (bs) {
      return {
        id: bs.id,
        name: bs.name || bs.id,
        meta: [bs.service_type, bs.status].filter(Boolean).join(' · '),
      };
    });
    el.innerHTML = _renderRelatedList(rows, 'No business services use this capability');
    el.querySelectorAll('.services-related-row').forEach(function (row, idx) {
      var bs = services[idx];
      if (bs && bs.id) {
        row.style.cursor = 'pointer';
        row.addEventListener('click', function () {
          var svc = _services();
          if (svc && typeof svc.showRecord === 'function') svc.showRecord(bs.id);
        });
      }
    });
  }

  function renderAIBindingsSection(capId) {
    var el = document.getElementById('capabilities-record-ai-bindings');
    if (!el) return;
    if (_aiLoading[capId]) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading AI bindings…</div>';
      return;
    }
    var err = _aiError[capId];
    if (err) {
      var status = err.status ? ('HTTP ' + err.status) : 'Network error';
      el.innerHTML =
        '<div class="capabilities-record-error">' +
          '<div><strong>Could not load AI bindings</strong></div>' +
          '<div>' + _escHtml(status) + (err.message ? ' — ' + _escHtml(err.message) : '') + '</div>' +
        '</div>';
      return;
    }
    var bindings = _aiCache[capId];
    if (!Array.isArray(bindings)) {
      el.innerHTML = '<div class="capabilities-record-loading">Loading AI bindings…</div>';
      return;
    }
    var rows = bindings.map(function (b) {
      var verSuffix = b.ai_system_version != null ? '·v' + b.ai_system_version : '';
      var meta = [b.role, b.ai_system_id ? 'system=' + b.ai_system_id + verSuffix : ''].filter(Boolean).join(' · ');
      return { id: b.id, name: b.role || b.id, meta: meta };
    });
    el.innerHTML = _renderRelatedList(rows, 'No AI bindings at this capability scope');
  }

  function init(options) {
    options = options || {};
    var search = document.getElementById('capabilities-search');
    if (search) search.addEventListener('input', function (e) { renderCatalogue(e.target.value); });
    var back = document.getElementById('capabilities-record-back-btn');
    if (back) back.addEventListener('click', showCatalogue);
  }

  window.MIDASExplorerCapabilities = {
    init:                     init,
    loadCatalogue:            loadCatalogue,
    showCatalogue:            showCatalogue,
    showRecord:               showRecord,
    renderCatalogue:          renderCatalogue,
    renderRecord:             renderRecord,
    loadRecord:               loadRecord,
    setSubView:               setSubView,
    getSubView:               getSubView,
    getSelectedCapabilityId:  getSelectedCapabilityId,
    setSelectedCapabilityId:  setSelectedCapabilityId,
    // Section render entry-points (used by reactive load handlers).
    renderChildrenSection:        renderChildrenSection,
    renderBusinessServicesSection: renderBusinessServicesSection,
    renderAIBindingsSection:      renderAIBindingsSection,
    // Sub-resource loaders (used by inline compatibility shims).
    loadChildren:                 loadChildren,
    loadBusinessServices:         loadBusinessServices,
    loadAIBindings:               loadAIBindings,
  };
})();
