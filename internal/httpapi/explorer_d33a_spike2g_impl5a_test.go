package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5a_test.go — D33a-spike-2g-impl-5a.
//
// Fix for the impl-5 same-render lifecycle bug: the PoC was
// inserting carrier DOM BEFORE `_destroyCy()`, and `_destroyCy()`
// calls `_clearInspectorCarriers()` as part of its teardown — so
// the carriers were wiped out moments after being inserted and the
// production inspector never found a target. The fix reorders
// `_renderPayload` so `_renderInspectorCarriers(elements)` runs
// AFTER `_destroyCy()`, ensuring carriers survive to the tap-handler
// routing.

const (
	d33aSpike2gImpl5aPocPath = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// d33aSpike2gImpl5aRenderPayloadBody returns the source of
// `_renderPayload(payload)` so the order-of-operations assertion
// is scoped to the function.
func d33aSpike2gImpl5aRenderPayloadBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl5aPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5a: cannot read PoC: %v", err)
	}
	js := string(b)
	start := strings.Index(js, "function _renderPayload(payload) {")
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5a: _renderPayload missing")
	}
	end := strings.Index(js[start+1:], "\n  function ")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5a: could not bound _renderPayload body")
	}
	return js[start : start+1+end]
}

// TestExplorer_D33aSpike2gImpl5a_CarrierRenderAfterDestroy pins that
// `_renderInspectorCarriers(elements)` is called AFTER `_destroyCy()`
// inside `_renderPayload`. If the order were reversed (impl-5's
// original bug), `_destroyCy()` would call
// `_clearInspectorCarriers()` and wipe out the just-inserted
// carriers, leaving the production inspector with nothing to find.
func TestExplorer_D33aSpike2gImpl5a_CarrierRenderAfterDestroy(t *testing.T) {
	body := d33aSpike2gImpl5aRenderPayloadBody(t)

	destroyIdx := strings.Index(body, "_destroyCy();")
	if destroyIdx < 0 {
		t.Fatal("D33a-spike-2g-impl-5a: `_destroyCy();` call missing from _renderPayload")
	}
	carrierIdx := strings.Index(body, "_renderInspectorCarriers(elements);")
	if carrierIdx < 0 {
		t.Fatal("D33a-spike-2g-impl-5a: `_renderInspectorCarriers(elements);` call missing from _renderPayload")
	}
	if !(carrierIdx > destroyIdx) {
		t.Errorf("D33a-spike-2g-impl-5a: carriers must be inserted AFTER _destroyCy() — found _renderInspectorCarriers at %d, _destroyCy at %d (impl-5 bug: same-render teardown wiped the carriers)", carrierIdx, destroyIdx)
	}
}

// TestExplorer_D33aSpike2gImpl5a_CarrierAndTeardownContractIntact pins
// the two related lifecycle invariants:
//   - `_renderInspectorCarriers` still calls `_clearInspectorCarriers`
//     at its head (idempotent before-render cleanup).
//   - `_uninstallPoc` still clears carriers (full teardown).
// These cleanups remain correct for their respective semantics; only
// the same-render ordering inside `_renderPayload` changed.
func TestExplorer_D33aSpike2gImpl5a_CarrierAndTeardownContractIntact(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl5aPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5a: cannot read PoC: %v", err)
	}
	js := string(b)

	// _renderInspectorCarriers clears before insert.
	rStart := strings.Index(js, "function _renderInspectorCarriers(")
	rEnd := strings.Index(js[rStart:], "\n  }")
	if !strings.Contains(js[rStart:rStart+rEnd], "_clearInspectorCarriers()") {
		t.Error("D33a-spike-2g-impl-5a: _renderInspectorCarriers must still clear stale carriers at the head of the helper")
	}

	// _uninstallPoc clears as part of the full teardown.
	uStart := strings.Index(js, "function _uninstallPoc(")
	uEnd := strings.Index(js[uStart:], "\n  }")
	if !strings.Contains(js[uStart:uStart+uEnd], "_clearInspectorCarriers()") {
		t.Error("D33a-spike-2g-impl-5a: _uninstallPoc must still call _clearInspectorCarriers() for the full PoC teardown")
	}
}
