package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37r-tranche-B''-centring-fix — Strategic Overlay Centring Contract
//
// The shared overlay module owns wrapper geometry and centring. The lens
// returns DOM; the engine measures it and positions it. The engine MUST
// NOT rely on the lens card's CSS positioning mode, dimensions, or root
// element type for centring.
//
// Pre-fix bug (confirmed by browser evidence + source diagnostic):
//
//   The wrapper had no explicit dimensions. When the lens card root was
//   `position: absolute` (as is the case for Context's `.gmap-node`
//   button, which has `position: absolute; width: 220px`), the wrapper
//   collapsed to 0 × 0 because absolutely-positioned descendants do
//   not contribute to a parent's shrink-to-fit width. The previous
//   `translate(-50%, -50%)` centring tail then translated by 0, leaving
//   cards top-left-aligned to cy edge endpoints instead of centred —
//   a uniform ~(110, 32) px offset across every card.
//
// Post-fix strategic contract:
//
//   1. The wrapper has NO explicit width / height. The wrapper's box
//      never participates in centring.
//   2. The engine measures the inner card's footprint synchronously
//      via getBoundingClientRect AFTER appending the wrapper to the
//      layer (so the browser has resolved the inner DOM's layout).
//   3. Measured dimensions are cached on the CardEntry record stored
//      in `_byKey`.
//   4. A per-card ResizeObserver invalidates the cached dimensions
//      when the inner card's content changes shape (selection border,
//      hover padding, dynamic content) and triggers a re-sync of that
//      one card.
//   5. The sync transform is translate3d(p.x - w/2, p.y - h/2, 0)
//      using EXPLICIT pixel arithmetic, NOT translate(-50%, -50%).
//   6. When ResizeObserver is unavailable (legacy hosts), the sync
//      re-measures each card on every pass — correctness over
//      performance.
//
// Pinned by source-contract tests in this file. The contract is
// documented in source at the head of `_wrapElement` in
// graph-cytoscape-overlay.js.

const overlayCentringAsset = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"

// ── 1. Measures inner card dimensions on mount ────────────────────

// TestExplorer_OverlayCentring_MeasuresInnerCardDimensions pins that
// `_build` (or equivalent) calls `getBoundingClientRect` (or an
// equivalent measurement API) on the inner card element after the
// wrapper is appended to the layer, and that the measured dimensions
// are cached on the CardEntry record.
func TestExplorer_OverlayCentring_MeasuresInnerCardDimensions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	// _measureCard helper must be present and use getBoundingClientRect.
	if !strings.Contains(js, "function _measureCard(entry)") {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must declare _measureCard(entry) helper")
	}
	if !regexp.MustCompile(`(?s)function _measureCard\(entry\)[\s\S]*?entry\.inner\.getBoundingClientRect\(\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _measureCard must read entry.inner.getBoundingClientRect() to size the card")
	}

	// The measured dimensions must be cached on the entry record.
	if !regexp.MustCompile(`entry\.measuredWidth\s*=`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _measureCard must cache the width as entry.measuredWidth")
	}
	if !regexp.MustCompile(`entry\.measuredHeight\s*=`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _measureCard must cache the height as entry.measuredHeight")
	}

	// _build must call _measureCard AFTER appending the wrapper to the
	// layer (so the browser has laid the inner DOM out). Verified by
	// scanning the _build body for the appendChild → _measureCard
	// sequence.
	bIdx := strings.Index(js, "function _build()")
	if bIdx < 0 {
		t.Fatal("D37r-tranche-B''-centring-fix: function _build() must exist")
	}
	bTail := js[bIdx:]
	bEndRel := strings.Index(bTail[1:], "\n    function ")
	if bEndRel < 0 {
		t.Fatalf("D37r-tranche-B''-centring-fix: _build body must be well-formed")
	}
	bBody := bTail[:bEndRel+1]

	appendIdx := strings.Index(bBody, "_layerEl.appendChild(entry.wrapper)")
	measureIdx := strings.Index(bBody, "_measureCard(entry)")
	if appendIdx < 0 {
		t.Errorf("D37r-tranche-B''-centring-fix: _build must append entry.wrapper to _layerEl")
	}
	if measureIdx < 0 {
		t.Errorf("D37r-tranche-B''-centring-fix: _build must call _measureCard(entry) to populate dimensions")
	}
	if appendIdx >= 0 && measureIdx >= 0 && appendIdx >= measureIdx {
		t.Errorf("D37r-tranche-B''-centring-fix: _measureCard MUST be called AFTER _layerEl.appendChild — measurement is invalid before the wrapper is in the document (appendIdx=%d, measureIdx=%d)", appendIdx, measureIdx)
	}
}

// ── 2. Transform uses explicit pixel arithmetic ───────────────────

// TestExplorer_OverlayCentring_TransformUsesExplicitArithmetic pins
// that the sync transform uses model-space translate(p.x - w/2,
// p.y - h/2) with inner card dimensions, NOT translate(-50%, -50%)
// against the wrapper's intrinsic box.
func TestExplorer_OverlayCentring_TransformUsesExplicitArithmetic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	// _syncCard must read model-space inner dimensions and apply
	// explicit-arithmetic centring.
	scIdx := strings.Index(js, "function _syncCard(entry)")
	if scIdx < 0 {
		t.Fatal("D37r-tranche-B''-centring-fix: function _syncCard(entry) must exist")
	}
	scTail := js[scIdx:]
	scEndRel := strings.Index(scTail[1:], "\n    function ")
	if scEndRel < 0 {
		t.Fatalf("D37r-tranche-B''-centring-fix: _syncCard body must be well-formed")
	}
	scBody := scTail[:scEndRel+1]

	if !strings.Contains(scBody, "var dims = _cardModelDimensions(entry);") {
		t.Errorf("D37p authority-projection: _syncCard must read model-space inner dimensions via _cardModelDimensions(entry)")
	}
	if !regexp.MustCompile(`var w = dims\.width;`).MatchString(scBody) {
		t.Errorf("D37p authority-projection: _syncCard must read width from dims.width")
	}
	if !regexp.MustCompile(`var h = dims\.height;`).MatchString(scBody) {
		t.Errorf("D37p authority-projection: _syncCard must read height from dims.height")
	}

	// Centring computation: tx = round(p.x - w/2), ty = round(p.y - h/2).
	if !regexp.MustCompile(`var tx = Math\.round\(p\.x\s*-\s*w\s*/\s*2\);`).MatchString(scBody) {
		t.Errorf("D37r-tranche-B''-centring-fix: _syncCard must compute tx = Math.round(p.x - w/2) for centring")
	}
	if !regexp.MustCompile(`var ty = Math\.round\(p\.y\s*-\s*h\s*/\s*2\);`).MatchString(scBody) {
		t.Errorf("D37r-tranche-B''-centring-fix: _syncCard must compute ty = Math.round(p.y - h/2) for centring")
	}

	// Transform writes translate with pre-computed model-space offsets (NO
	// translate(-50%, -50%) tail in the load-bearing centring write).
	if !regexp.MustCompile(`entry\.wrapper\.style\.transform\s*=\s*'translate\(' \+ tx \+ 'px, ' \+ ty \+ 'px\)'`).MatchString(scBody) {
		t.Errorf("D37p authority-projection: _syncCard's centring transform must be translate(tx px, ty px)")
	}

	// Negative pin — translate(-50%, -50%) is absent from _syncCard.
	if strings.Contains(scBody, "translate(-50%, -50%)") {
		t.Errorf("D37r-tranche-B''-centring-fix: _syncCard must NOT use translate(-50%%, -50%%) — that tail silently translates by zero when the wrapper has no intrinsic dimensions (which is the pre-fix bug shape)")
	}
}

// ── 3. Per-card ResizeObserver ────────────────────────────────────

// TestExplorer_OverlayCentring_PerCardResizeObserver pins that the
// shared module instantiates a ResizeObserver per card observing the
// inner element, and disconnects every per-card observer on destroy
// and refresh.
func TestExplorer_OverlayCentring_PerCardResizeObserver(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	// _observeCard helper must be present and instantiate a
	// ResizeObserver on entry.inner.
	if !strings.Contains(js, "function _observeCard(entry)") {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must declare _observeCard(entry) helper")
	}
	if !regexp.MustCompile(`(?s)function _observeCard\(entry\)[\s\S]*?new window\.ResizeObserver`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _observeCard must instantiate `new window.ResizeObserver(...)`")
	}
	if !regexp.MustCompile(`(?s)function _observeCard\(entry\)[\s\S]*?ro\.observe\(entry\.inner\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _observeCard must call ro.observe(entry.inner) to track the lens-supplied card")
	}

	// The observer callback must re-sync that card when dimensions
	// change (so dynamic content updates re-centre).
	if !regexp.MustCompile(`(?s)function _observeCard\(entry\)[\s\S]*?_syncCard\(entry\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _observeCard's callback must call _syncCard(entry) when dimensions change")
	}

	// Disconnect helper for a single entry.
	if !strings.Contains(js, "function _disconnectCardObserver(entry)") {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must declare _disconnectCardObserver(entry)")
	}
	if !regexp.MustCompile(`entry\.resizeObserver\.disconnect\(\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _disconnectCardObserver must call entry.resizeObserver.disconnect()")
	}

	// Bulk disconnect helper exists and is called from destroy +
	// refresh so per-card observers don't outlive their entries.
	if !strings.Contains(js, "function _disconnectAllCardObservers()") {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must declare _disconnectAllCardObservers()")
	}
	dIdx := strings.Index(js, "function destroy()")
	if dIdx < 0 {
		t.Fatal("D37r-tranche-B''-centring-fix: destroy() must exist")
	}
	dTail := js[dIdx:]
	dEndRel := strings.Index(dTail[1:], "\n    function ")
	if dEndRel < 0 {
		t.Fatalf("D37r-tranche-B''-centring-fix: destroy body must be well-formed")
	}
	dBody := dTail[:dEndRel+1]
	if !strings.Contains(dBody, "_disconnectAllCardObservers()") {
		t.Errorf("D37r-tranche-B''-centring-fix: destroy() must call _disconnectAllCardObservers() so per-card observers can't fire on stale entries")
	}

	rIdx := strings.Index(js, "function refresh()")
	if rIdx < 0 {
		t.Fatal("D37r-tranche-B''-centring-fix: refresh() must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n    function ")
	if rEndRel < 0 {
		t.Fatalf("D37r-tranche-B''-centring-fix: refresh body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]
	if !strings.Contains(rBody, "_disconnectAllCardObservers()") {
		t.Errorf("D37r-tranche-B''-centring-fix: refresh() must call _disconnectAllCardObservers() before rebuilding")
	}
}

// ── 4. ResizeObserver-unavailable fallback ────────────────────────

// TestExplorer_OverlayCentring_ResizeObserverFallback pins that the
// shared module captures ResizeObserver availability once and falls
// back to re-measuring on every sync when unavailable.
func TestExplorer_OverlayCentring_ResizeObserverFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	// Module captures `typeof window.ResizeObserver === 'function'`
	// once at mount time into a flag.
	if !regexp.MustCompile(`var _hasResizeObserver\s*=\s*\(typeof window\.ResizeObserver === 'function'\);`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must capture ResizeObserver availability into _hasResizeObserver")
	}

	// _observeCard must early-return when ResizeObserver is unavailable
	// (the per-card observer cannot be attached, so the fallback
	// path takes over).
	if !regexp.MustCompile(`(?s)function _observeCard\(entry\)\s*\{\s*if \(!_hasResizeObserver`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: _observeCard must early-return when !_hasResizeObserver")
	}

	// _cardModelDimensions' fallback path re-measures when dimensions
	// are stale OR when _hasResizeObserver is false. The fallback
	// condition must reference _hasResizeObserver.
	scIdx := strings.Index(js, "function _cardModelDimensions(entry)")
	if scIdx < 0 {
		t.Fatal("D37p authority-projection: _cardModelDimensions must exist")
	}
	scTail := js[scIdx:]
	scEndRel := strings.Index(scTail[1:], "\n    function ")
	if scEndRel < 0 {
		t.Fatalf("D37p authority-projection: _cardModelDimensions body must be well-formed")
	}
	scBody := scTail[:scEndRel+1]
	if !strings.Contains(scBody, "!_hasResizeObserver") {
		t.Errorf("D37p authority-projection: _cardModelDimensions must re-measure when !_hasResizeObserver (fallback path for legacy hosts)")
	}
	if !strings.Contains(scBody, "_measureCard(entry)") {
		t.Errorf("D37p authority-projection: _cardModelDimensions fallback path must call _measureCard(entry) to refresh dimensions")
	}
}

// ── 5. Accepts position:absolute templates without mutation ───────

// TestExplorer_OverlayCentring_SupportsAbsolutePositionedTemplate
// pins (via structural assertion) that the overlay does NOT mutate
// `inner.style.position`. The engine MUST accept lens templates with
// any positioning mode — the bug that motivated this tranche was
// rooted in the wrapper-vs-inner CSS positioning interaction, and the
// fix is to make the engine measurement-based rather than
// CSS-positioning-mode-dependent.
func TestExplorer_OverlayCentring_SupportsAbsolutePositionedTemplate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	// The overlay module's source must not write to inner.style.position.
	// This is the strategic contract invariant — the engine accepts
	// whatever positioning mode the lens template uses.
	for _, banned := range []string{
		"inner.style.position",
		"inner.style.left",
		"inner.style.top",
		"inner.style.right",
		"inner.style.bottom",
		"inner.style.transform",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37r-tranche-B''-centring-fix: shared module must NOT mutate the inner element's positioning property (%q found in source) — the engine is the sole positioner; the lens template's CSS positioning mode is opaque to the engine", banned)
		}
	}

	// The wrapper, by contrast, IS positioned by the engine.
	if !regexp.MustCompile(`wrapper\.style\.position\s*=\s*'absolute';`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: wrapper MUST be position:absolute (the engine positions the wrapper, not the inner card)")
	}

	// The wrapper has NO explicit width / height set in source — its
	// box never participates in the centring arithmetic, only the
	// measured inner does.
	if regexp.MustCompile(`wrapper\.style\.width\s*=`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: wrapper MUST NOT have an explicit width — centring is driven by measured inner dimensions, not wrapper intrinsic size")
	}
	if regexp.MustCompile(`wrapper\.style\.height\s*=`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix: wrapper MUST NOT have an explicit height — centring is driven by measured inner dimensions, not wrapper intrinsic size")
	}
}

// ── 6. Strategic centring contract documented ─────────────────────

// TestExplorer_OverlayCentring_StrategicRuleDocumented pins that the
// STRATEGIC OVERLAY CENTRING CONTRACT comment block is present at the
// head of _wrapElement and names the load-bearing rule.
func TestExplorer_OverlayCentring_StrategicRuleDocumented(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, overlayCentringAsset)

	if !strings.Contains(js, "STRATEGIC OVERLAY CENTRING CONTRACT") {
		t.Errorf("D37r-tranche-B''-centring-fix: shared module must carry the STRATEGIC OVERLAY CENTRING CONTRACT documentation block")
	}

	// The contract must name the load-bearing rule.
	for _, phrase := range []string{
		"MUST NOT rely on the lens card's CSS positioning mode",
		"EXPLICIT PIXEL ARITHMETIC",
		"translate(-50%, -50%) pattern is",
	} {
		if !strings.Contains(js, phrase) {
			t.Errorf("D37r-tranche-B''-centring-fix: strategic contract block must include %q", phrase)
		}
	}

	// The contract block must appear AT or BEFORE _wrapElement (it
	// documents that function's invariants).
	contractIdx := strings.Index(js, "STRATEGIC OVERLAY CENTRING CONTRACT")
	wrapIdx := strings.Index(js, "function _wrapElement(node)")
	if contractIdx < 0 || wrapIdx < 0 {
		t.Fatal("D37r-tranche-B''-centring-fix: contract block and _wrapElement must both be present")
	}
	if contractIdx >= wrapIdx {
		t.Errorf("D37r-tranche-B''-centring-fix: strategic contract block must precede _wrapElement (contractIdx=%d, wrapIdx=%d)", contractIdx, wrapIdx)
	}
}
