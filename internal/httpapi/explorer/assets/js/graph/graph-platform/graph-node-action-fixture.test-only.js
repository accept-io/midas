// /explorer/assets/js/graph/graph-platform/graph-node-action-fixture.test-only.js
//
// Test-only fixture for source and harness validation. This file is not loaded
// by index.html and must not register actions for shipping lenses.
(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  var registry = window.MIDASExplorerGraph && window.MIDASExplorerGraph.nodeActionRegistry;
  if (!registry || typeof registry.registerActions !== 'function') return;
  registry.registerActions({
    lensId: '__fixture__',
    nodeKind: '__fixture__',
    actions: [
      {
        id: 'fixture-noop',
        label: 'Fixture action',
        run: function () {},
      },
    ],
  });
})();
