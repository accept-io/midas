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

  function _demoSourceClassification(extra) {
    var vm = _vm();
    var base = Object.assign({}, (vm && vm.demoSourceClassification) || {
      observedSeries: 'demo_fallback',
      expectedBaseline: 'demo_fallback',
      thresholds: 'demo_fallback',
      status: 'demo_fallback',
      provenance: 'demo',
      compositeScore: 'demo_provisional',
      contributionValues: 'demo_provisional',
      contributionWeights: 'demo_provisional',
      graphOverlay: 'not_implemented'
    });
    return Object.assign(base, extra || {});
  }

  function fromServiceContext(opts) {
    opts = opts || {};
    var vm = _vm();
    var base = {
      serviceLabel: _contextLabel('Service', opts.serviceId),
      nodeLabel: opts.serviceId ? 'Service ' + opts.serviceId : 'Node bs:bs-cards',
      sourceClassification: _demoSourceClassification(),
      sourceStateLabel: opts.sourceStateLabel || 'Demo evidence',
      isDemo: true
    };
    return vm && typeof vm.normalise === 'function' ? vm.normalise(base) : base;
  }

  function fromGraphNode(opts) {
    opts = opts || {};
    var vm = _vm();
    var base = {
      nodeLabel: opts.nodeId ? 'Node ' + opts.nodeId : 'Node bs:bs-cards',
      sourceClassification: _demoSourceClassification(opts.sourceClassification),
      sourceStateLabel: opts.sourceStateLabel || 'Demo evidence',
      isDemo: true
    };
    return vm && typeof vm.normalise === 'function' ? vm.normalise(base) : base;
  }

  function isDemoData(result) {
    return !!(result && (result.isDemo ||
      (result.sourceClassification &&
       result.sourceClassification.observedSeries === 'demo_fallback')));
  }

  window.MIDASExplorerDriftChartAdapter = {
    fromServiceContext: fromServiceContext,
    fromGraphNode: fromGraphNode,
    isDemoData: isDemoData
  };
})();
