// /explorer/assets/js/drift/drift-series-list.js - D32e-tranche-1
//
// Contribution rail renderer for the Drift Analytics panel.

(function () {
  'use strict';

  function _escText(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  function _escAttr(s) {
    return _escText(s).replace(/"/g, '&quot;');
  }

  function _rowHTML(item, selected) {
    var id = item.id || '';
    var label = item.label || id || 'Contribution';
    var color = item.color || 'grey';
    return '<button type="button" class="drift-contribution-row drift-contribution-row-' + _escAttr(color) +
      (selected ? ' is-selected' : '') + '"' +
      ' data-drift-contribution-id="' + _escAttr(id) + '"' +
      ' aria-pressed="' + (selected ? 'true' : 'false') + '"' +
      ' aria-label="' + _escAttr(label + ' contribution ' + (item.value || '') + ' ' + (item.share || '')) + '">' +
        '<span class="drift-contribution-label">' + _escText(label) + '</span>' +
        '<span class="drift-contribution-value">' + _escText(item.value || '') + '</span>' +
        '<span class="drift-contribution-share">' + _escText(item.share || '') + '</span>' +
        '<span class="drift-contribution-track" aria-hidden="true">' +
          '<span class="drift-contribution-fill" style="width:' + _escAttr(String(parseInt(item.share, 10) || 0)) + '%"></span>' +
        '</span>' +
      '</button>';
  }

  function render(mount, viewModel, options) {
    if (!mount) return;
    options = options || {};
    var contributions = viewModel && Array.isArray(viewModel.contributions) ? viewModel.contributions : [];
    var selectedId = options.selectedId || (viewModel && viewModel.selectedContributionId) || '';
    mount.setAttribute('role', 'list');
    mount.setAttribute('aria-label', options.ariaLabel || 'Drift contribution rail');
    if (contributions.length === 0) {
      mount.innerHTML = '<div class="drift-contribution-empty" role="status">No drift contributions.</div>';
      return;
    }
    mount.innerHTML = contributions.map(function (item) {
      return _rowHTML(item, item.id === selectedId);
    }).join('');
    if (mount._driftContributionHandler) {
      mount.removeEventListener('click', mount._driftContributionHandler);
      mount._driftContributionHandler = null;
    }
    if (typeof options.onSelect === 'function') {
      mount._driftContributionHandler = function (ev) {
        var target = ev.target;
        while (target && target !== mount && !(target.classList && target.classList.contains('drift-contribution-row'))) {
          target = target.parentNode;
        }
        if (!target || target === mount) return;
        var id = target.getAttribute('data-drift-contribution-id');
        if (id) options.onSelect(id);
      };
      mount.addEventListener('click', mount._driftContributionHandler);
    }
  }

  function clear(mount) {
    if (!mount) return;
    if (mount._driftContributionHandler) {
      mount.removeEventListener('click', mount._driftContributionHandler);
      mount._driftContributionHandler = null;
    }
    mount.innerHTML = '';
  }

  window.MIDASExplorerDriftContributionRail = {
    render: render,
    clear: clear
  };
})();
