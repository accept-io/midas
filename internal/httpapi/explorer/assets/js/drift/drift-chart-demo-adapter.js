// /explorer/assets/js/drift/drift-chart-demo-adapter.js - D32e-tranche-1
//
// Demo producer for the Drift Analytics view model. It does not fetch
// and does not imply runtime authority; the panel labels its output as
// demo-derived until a real derive adapter is wired.

(function () {
  'use strict';

  function _vm() {
    return window.MIDASExplorerDriftAnalyticsViewModel || null;
  }

  function _contextLabel(prefix, id) {
    return id ? prefix + ' ' + id : 'Cards';
  }

  function fromServiceContext(opts) {
    opts = opts || {};
    var vm = _vm();
    var base = {
      serviceLabel: _contextLabel('Service', opts.serviceId),
      nodeLabel: opts.serviceId ? 'Service ' + opts.serviceId : 'Node bs:bs-cards',
      sourceClassification: 'demo_derived',
      isDemo: true
    };
    return vm && typeof vm.normalise === 'function' ? vm.normalise(base) : base;
  }

  function fromGraphNode(opts) {
    opts = opts || {};
    var vm = _vm();
    var base = {
      nodeLabel: opts.nodeId ? 'Node ' + opts.nodeId : 'Node bs:bs-cards',
      sourceClassification: 'demo_derived',
      isDemo: true
    };
    return vm && typeof vm.normalise === 'function' ? vm.normalise(base) : base;
  }

  function isDemoData(result) {
    return !!(result && result.sourceClassification === 'demo_derived');
  }

  window.MIDASExplorerDriftChartAdapter = {
    fromServiceContext: fromServiceContext,
    fromGraphNode: fromGraphNode,
    isDemoData: isDemoData
  };
})();
