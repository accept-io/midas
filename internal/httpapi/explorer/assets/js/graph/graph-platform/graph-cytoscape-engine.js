// /explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js
//
// D37r-tranche-B'' — Shared Graph Engine Platform Module.
//
// ONE Cytoscape instantiation. ONE mount lifecycle. ONE coordinate
// frame. ONE overlay alignment mechanism. ONE bus-registration path
// for camera control. Lenses (Authority, Context, Knowledge, future
// graph types) supply data, templates, and adapters; the engine owns
// the engine.
//
// ── Strategic rule (load-bearing) ─────────────────────────────────
//
//   NO GRAPH LENS MAY INSTANTIATE CYTOSCAPE DIRECTLY.
//   NO GRAPH LENS MAY MOUNT OR MANAGE THE HTML OVERLAY LAYER
//   DIRECTLY. LENSES SUPPLY DATA, TEMPLATES, AND ADAPTERS TO THIS
//   ENGINE. THE ENGINE OWNS CYTOSCAPE INSTANTIATION, MOUNT
//   CONTAINER MANAGEMENT, COORDINATE FRAME, OVERLAY ALIGNMENT,
//   SCRIPT-LOAD HANDLING, LIFECYCLE, AND EVENT PLUMBING.
//
// This rule is encoded as a source-contract test in
// `internal/httpapi/explorer_graph_cytoscape_engine_test.go`
// (`TestExplorer_StrategicRule_NoLensInstantiatesCytoscape`) which
// scans every JS file outside the engine module for direct
// `window.cytoscape({...})` constructor calls and for direct
// overlay-layer construction patterns. Future lenses cannot
// re-fork the engine.
//
// ── Transitional whitelist note ────────────────────────────────────
//
// At the time of this tranche (B''), Authority's `authority-cytoscape-
// poc.js` retains its own direct `window.cytoscape({...})` call site
// and its own bespoke fit / theme / edge-label-overlay mechanics.
// Authority is whitelisted by the strategic-rule test pending a
// follow-up Authority-migration tranche (B''-Authority). The
// whitelist entry MUST be removed before tranche E (Context default
// flip) can land — Authority must consume the engine before any
// default-path graph lens does.
//
// ── Public API ─────────────────────────────────────────────────────
//
// window.MIDASExplorerGraph.graphCytoscapeEngine = {
//   mount(mountEl, options) → handle
// }
//
//   mountEl  — the DOM element the engine will host its container,
//              cy canvases, and overlay inside. The engine creates
//              an internal container DIV as the SOLE direct child of
//              `mountEl` and sizes it to fill its parent. Callers
//              must ensure `mountEl` has a non-zero visible size at
//              mount time.
//
//   options  — required keys: `lensId`, `data`, `template`,
//              `keyForNode`, `selectionAdapter`, `cameraAdapter`.
//              Optional keys: `stateClasses`, `syncSelected`,
//              `syncHover`, `pointerEvents`, `layerClassName`,
//              `nodeStyleOverride`.
//
//     lensId            — string, required. Diagnostic identifier
//                         carried on the engine container's
//                         `data-lens` attribute AND used to
//                         register the lens's camera-bus delegate
//                         via `graphCameraBus.registerLens(lensId,
//                         delegate)`.
//     data              — `{ nodes: [...], edges: [...] }` in the
//                         canonical engine shape. See "Canonical
//                         data shape" below.
//     template          — `{ create(node, ctx) → DOMElement,
//                            update?(el, node, ctx),
//                            className?(node) → string }`. Passed
//                         through to the internal overlay module
//                         which calls `template.create` per cy node
//                         to produce the visible card DOM.
//     keyForNode        — `(node) → string`. Per-cy-node key
//                         extractor used by the overlay to track
//                         wrapper↔node correspondence. Typically
//                         `function (n) { return n.id(); }`.
//     selectionAdapter  — `(cyEvent, handle) → void`. Engine
//                         subscribes to `cy.on('tap', 'node', …)`
//                         and invokes this adapter on every node
//                         tap. The adapter routes selection through
//                         whatever bridge the lens uses
//                         (e.g. `contextSelectionBridge.selectCard`).
//                         The engine does NOT itself publish to any
//                         lens selection bridge or to
//                         `graphSelectionBridge` — the lens owns
//                         that wiring.
//     cameraAdapter     — `(handle) → cameraDelegate`. Lens-supplied
//                         factory that, given the engine's handle,
//                         returns a delegate object conforming to
//                         the `graphCameraBus` locked vocabulary
//                         (`zoomIn`, `zoomOut`, `fit`, `reset`,
//                         `setZoom`, `getZoom`, `focusRoot`,
//                         `focusSelected`). The engine calls the
//                         adapter immediately at mount time and
//                         registers the returned delegate with
//                         `graphCameraBus` under `lensId`. The
//                         delegate's implementation calls back into
//                         the handle's standardised methods (e.g.
//                         `handle.zoomIn()`, `handle.focus(id)`) —
//                         the engine owns the cy-touching code.
//
//     stateClasses      — `{ selected, hover }` object specifying
//                         the class names applied to the overlay
//                         card on cy `select` / `mouseover` events.
//                         Default `{ selected: 'is-selected',
//                         hover: 'is-hover' }`. Passed through to
//                         the internal overlay module.
//     syncSelected      — boolean, default true. Forwarded to the
//                         overlay's `syncSelected`. When true, cy
//                         select/unselect events toggle
//                         `stateClasses.selected` on overlay cards.
//     syncHover         — boolean, default true. Forwarded to the
//                         overlay's `syncHover`. When true, cy
//                         mouseover/mouseout events toggle
//                         `stateClasses.hover` on overlay cards.
//     pointerEvents     — string, default `'none'`. Forwarded to
//                         the overlay's `pointerEvents`. Controls
//                         the wrapper + template-returned
//                         element's `pointer-events` CSS value.
//                         Use `'none'` for "cy gets every click"
//                         (Context); use `'auto'` for "cards are
//                         clickable" (Authority, when Authority
//                         migrates).
//     layerClassName    — optional string. Additional class names
//                         appended to the overlay layer DIV's
//                         className. Lenses with legacy CSS rules
//                         targeting a specific class on the
//                         overlay layer pass that class here.
//     nodeStyleOverride — optional array of Cytoscape style entries
//                         in the form `[{selector, style}, …]`.
//                         These entries are appended to the
//                         engine's base style array so lenses can
//                         add their own per-class node / edge
//                         styling (e.g. Context's five connector
//                         visual classes + dash semantics from
//                         tranche B). The engine's base style is
//                         documented in `_buildBaseStyle` below;
//                         overrides MAY introduce additional
//                         selectors but SHOULD NOT redefine the
//                         base `node` selector (the engine's
//                         transparent-node policy is invariant).
//     getSafeArea       — optional `() → {top, right, bottom, left}`
//                         function. Legacy fit input. The engine
//                         falls back to this when `getUsableGraphRect`
//                         (below) is not supplied. Composes the
//                         returned insets with `DEFAULT_FIT_PADDING`
//                         as a per-side floor.
//     getUsableGraphRect — optional `() → {x, y, width, height,
//                         insets}` function. D37s-viewport-fit-1-impl
//                         strategic fit-envelope contract. When
//                         supplied, the engine derives fit pan/zoom
//                         from the usable rectangle directly, NOT
//                         from getSafeArea-composed padding. This is
//                         the canonical input — GraphViewport's
//                         `getUsableGraphRect()` returns the actual
//                         usable graph area in viewport-relative
//                         coordinates, including chrome that lives
//                         OUTSIDE the viewport DOM tree (right
//                         drawer, etc.). When BOTH options are
//                         supplied, `getUsableGraphRect` wins;
//                         `getSafeArea` is consulted only as a
//                         fallback when `getUsableGraphRect` returns
//                         a zero rect.
//     overlayEnabled    — optional boolean, default `true`. When set
//                         to `false`, the engine skips the shared
//                         overlay-module mount entirely; only the
//                         raw cy canvas is rendered. Lenses that opt
//                         out are responsible for supplying a
//                         `nodeStyleOverride` that makes the raw cy
//                         nodes visible (the engine's base `node`
//                         style is transparent because it expects
//                         the overlay to paint the visible cards).
//                         Used in this build by strategic spatial
//                         Context for diagnostic inspection of the
//                         underlying graph; Authority is unaffected
//                         because Authority does not consume the
//                         shared overlay (it has its own bespoke
//                         `_installHtmlCardOverlay` path).
//     onMeasurementsChange — optional `(measurements) → void` callback.
//                         The engine invokes this callback (coalesced
//                         to one call per rAF) whenever the overlay
//                         reports a measurement change. `measurements`
//                         is a shallow-cloned `{ [nodeId]: {width,
//                         height} }` snapshot of the engine's current
//                         measurement cache. The lens consumes it to
//                         decide whether to reflow its stage with
//                         updated footprints (per its own threshold
//                         and scheduling policy). The engine does NOT
//                         recompose layout itself — the callback is
//                         the lens's hook to react to measured runtime
//                         dimensions. See the "Measurement-change
//                         forwarding" section of mount() and Context
//                         renderer's REFLOW GUARD CONTRACT
//                         (`_storeMeasuredFootprint`) for the loop
//                         termination argument.
//
// ── Canonical data shape (lens input) ──────────────────────────────
//
//   data.nodes: [
//     {
//       id:        string,                          // required, unique
//       position:  { x: number, y: number },        // required, preset coords
//       kind:      string,                          // optional, diagnostic
//       data:      object,                          // optional, merged into cy node data
//       classes:   string[] | string,               // optional, applied to cy node
//     },
//     …
//   ]
//
//   data.edges: [
//     {
//       id:           string,                       // required, unique
//       source:       string,                       // required, must match a node.id
//       target:       string,                       // required, must match a node.id
//       kind:         string,                       // optional, diagnostic
//       visualClass:  string,                       // optional, lens-class hint
//       dashPattern:  string | object | array,      // optional, lens-class hint
//       data:         object,                       // optional, merged into cy edge data
//       classes:      string[] | string,            // optional, applied to cy edge
//     },
//     …
//   ]
//
// Lenses translate their internal node/edge representations to this
// shape via a thin adapter inside the lens, then call
// `engine.mount(mountEl, { lensId, data, … })`. The translation is
// a lens concern; the canonical shape is the engine contract.
//
// ── Handle ─────────────────────────────────────────────────────────
//
// The handle returned by `mount(...)` exposes the engine's external
// surface. Lenses operate the engine through the handle ONLY; the
// internal cy instance is intentionally NOT exposed.
//
//   handle.destroy()                 — full teardown: overlay, cy
//                                      instance, listeners, observer,
//                                      container DOM, camera-bus
//                                      registration. Idempotent.
//   handle.refresh(data)             — replace nodes/edges with new
//                                      canonical data; preserve
//                                      camera state (zoom/pan);
//                                      recompute overlay cards.
//   handle.getCardEl(key) → element  — overlay card accessor (proxied
//                                      from the overlay's
//                                      `getCardEl(key)`).
//   handle.getNode(id) → descriptor  — engine descriptor
//                                      `{id, position, kind, data}`
//                                      for the cy node with the
//                                      given id. NOT the raw cy
//                                      node; lenses cannot reach
//                                      into cy through this surface.
//   handle.zoomIn() / zoomOut()      — bus-locked vocabulary
//                                      operations on cy's viewport.
//   handle.fit(opts?)                — safe-area-aware fit. `opts.padding`
//                                      accepts a scalar (uniform on all
//                                      four sides) OR a per-side
//                                      `{top, right, bottom, left}` object.
//                                      When omitted, the engine pulls
//                                      insets from `options.getSafeArea`
//                                      (mount option) and composes them
//                                      with DEFAULT_FIT_PADDING as a
//                                      per-side floor. Applies via
//                                      `cy.viewport({zoom, pan})` so
//                                      different chrome on different
//                                      sides (left rail, right drawer,
//                                      bottom tray, top toolbar) is
//                                      honoured independently. See
//                                      _fitWithSafeArea for algorithm.
//   handle.reset()                   — cy.zoom(1) + cy.center().
//   handle.focus(nodeId)             — cy.center(node).
//   handle.getZoom() → ratio         — cy.zoom() as a ratio.
//   handle.setZoom(level)            — clamped cy.zoom({level, …}).
//   handle.forceRender()             — explicit cy.forceRender().
//   handle.getDiagnostics() → array  — current engine-level diagnostics
//                                      buffer (overlap detections, etc.)
//                                      as a shallow-cloned array.
//                                      Each entry has `{code, cardA,
//                                      cardB, ts}`. Codes:
//                                        'card_overlap_detected' — two
//                                          cards' bounding boxes
//                                          overlap once measured runtime
//                                          dimensions are available.
//                                          De-dup'd per pair per
//                                          validation cycle; re-fires
//                                          if overlap re-emerges after
//                                          a no-overlap validation.
//
// ── Engine constraints ─────────────────────────────────────────────
//
//   • No reference to `authority`, `context`, `knowledge`, or any
//     specific lens identifier (lensId is treated as an opaque
//     string).
//   • No import or reference to lens-specific symbols
//     (`contextCardPainter`, `contextSelectionBridge`,
//     `cytoscapePoc`, `authority*`, etc.).
//   • No subscription to or publication on any lens selection
//     bridge.
//   • No CSS file owned by this module. Card-visible styling comes
//     from the lens's template + the lens's stylesheets.
//   • Cy instantiation requires `window.cytoscape` to be defined.
//     The vendor script tag is now positioned before this module in
//     `index.html`, so `window.cytoscape` is synchronously available
//     at mount time. The pre-tranche-B'' retry mechanism (Context's
//     `_cytoscapeAvailable` + `_scheduleCytoscapeRetry`) is deleted.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var CONTAINER_CLASS = 'graph-cytoscape-engine-container';
  var CY_MOUNT_CLASS  = 'graph-cytoscape-engine-cy-mount';

  // ── Fit / safe-area constants ──────────────────────────────────────
  //
  // The engine's fit owns the per-side safe-area-aware viewport
  // computation. Constants mirror Authority's `_fitToAvailableCanvas`
  // pattern (pending tranche B''-Authority migration onto this engine):
  //
  //   DEFAULT_FIT_PADDING       — uniform per-side floor applied when
  //                               neither opts.padding nor opts.getSafeArea
  //                               supplies a value. 24 px matches the
  //                               pre-tranche `cy.fit(undefined, 24)`
  //                               default for backward-compat surfaces.
  //   FIT_MIN_VISIBLE_PX        — degenerate-viewport guard. If the
  //                               per-side insets would collapse the
  //                               visible region below this threshold,
  //                               the insets are scaled back
  //                               proportionally so the graph stays
  //                               visible (mirrors Authority's
  //                               FIT_MIN_VISIBLE_PX = 96).
  //   FIT_ZOOM_MIN / MAX        — clamp the computed zoom into a
  //                               readable range. Cytoscape's defaults
  //                               (1e-50 .. 1e50) are unhelpful for
  //                               graph viewports of this size.
  var DEFAULT_FIT_PADDING = 24;
  var FIT_MIN_VISIBLE_PX  = 96;
  var FIT_ZOOM_MIN        = 0.05;
  var FIT_ZOOM_MAX        = 2.0;

  // ── Overlap-validation constant ───────────────────────────────────
  //
  // OVERLAP_VALIDATION_DEBOUNCE_MS — minimum interval (in ms) between
  // engine-level overlap-validation passes. The engine validates the
  // non-overlap invariant against MEASURED runtime card dimensions
  // (overlay-measured) and the cy nodes' MODEL positions (lens-
  // supplied). Validation runs at most once per debounce window even
  // if the overlay reports a burst of measurement changes within
  // that window. This is the dedup guard for the engine's
  // diagnostics — without it, a chatty overlay would emit a stream
  // of duplicate `card_overlap_detected` warnings every frame.
  var OVERLAP_VALIDATION_DEBOUNCE_MS = 250;

  // ── Fit-envelope diagnostic codes ─────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — diagnostics emitted by the engine's
  // fit pipeline. Surfaced via `handle.getDiagnostics()` and (de-
  // duplicated) console warns. Tests / dev tools key on these codes.
  var DIAG_USABLE_RECT_EMPTY            = 'usable_rect_empty';
  var DIAG_FIT_BOUNDS_EMPTY             = 'fit_bounds_empty';
  var DIAG_FIT_ZOOM_CLAMPED_MIN         = 'fit_zoom_clamped_min';
  var DIAG_FIT_ZOOM_CLAMPED_MAX         = 'fit_zoom_clamped_max';
  var DIAG_USABLE_RECT_SMALLER_THAN_MIN = 'usable_rect_smaller_than_minimum';
  // D37x-engine-node-geometry-contract — Emitted (dedup'd) when a
  // lens supplies a node without positive finite `data.width` and
  // `data.height`. Geometry is required: declared body dimensions
  // are the engine's source of truth for layout footprint and fit
  // math. Invalid dimensions produce a permissive fallback at the
  // mount path (cy will use its own defaults) but the operator and
  // tests see the violation in the diagnostics buffer.
  var DIAG_NODE_GEOMETRY_INVALID        = 'node_geometry_invalid_dimensions';
  // D37u-cytoscape-resize-policy-impl — Resize-only passive lifecycle.
  //
  // The shared graph engine owns resize lifecycle, camera policy, and
  // fit policy on behalf of every lens (Context, Authority post-
  // migration, Knowledge, drift, resilience, future workbench views).
  // The upstream Cytoscape assessment (D37t) established the correct
  // rule:
  //
  //   cy.resize() invalidates Cytoscape's cached container size and
  //   schedules a redraw; it does not mutate _private.pan or
  //   _private.zoom. cy.fit() and cy.viewport({zoom, pan}) DO mutate
  //   camera state. Passive container resizes (drawer toggle, tray
  //   expand, focus-mode chrome change, window resize) are NOT
  //   camera-mutation events; the operator's pan/zoom must survive.
  //
  // Policy:
  //
  //   • Passive resize path: cy.resize() only. No fit. No viewport.
  //     No deferred refit timer. Operator camera preserved.
  //   • Explicit fit path (handle.fit, _settle, future operator
  //     actions): allowed, source-tagged via `engine_fit_invoked`.
  //
  // The earlier trailing-edge debounce + previous-rect guard
  // (RESIZE_SETTLE_WINDOW_MS, _lastFittedRect, _rectMateriallyDiffers,
  // _runResizeRefit) was a refinement of the WRONG policy: it reduced
  // multiple incorrect refits to one delayed incorrect refit, which
  // the operator saw as a single longer flash. This tranche removes
  // that machinery entirely.
  //
  // Four codes report the lifecycle. Each emits PER FIRING (no
  // dedup) so external observers can count events.
  //
  //   engine_resize_detected             — ResizeObserver tick
  //                                        observed on the engine
  //                                        container.
  //   engine_cy_resize_called            — cy.resize() ran.
  //   engine_camera_preserved_on_resize  — passive resize completed
  //                                        without mutating pan/zoom.
  //   engine_fit_invoked                 — emitted by the explicit-
  //                                        fit code path with a
  //                                        `source` field naming the
  //                                        trigger (initial_mount,
  //                                        explicit_fit, reset_view,
  //                                        zoom_to_selected,
  //                                        lens_activation,
  //                                        data_reload).
  var DIAG_ENGINE_RESIZE_DETECTED            = 'engine_resize_detected';
  var DIAG_ENGINE_CY_RESIZE_CALLED           = 'engine_cy_resize_called';
  var DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE = 'engine_camera_preserved_on_resize';
  var DIAG_ENGINE_FIT_INVOKED                = 'engine_fit_invoked';
  // D37ah-readiness-gated-initial-fit — Readiness-gated initial fit.
  //
  // CLEAN BREAK from D37af. D37ab established hide-before-reveal;
  // D37ad added a multi-tick fit cadence; D37af replaced the fixed
  // cadence with a stability counter inside a time-bounded rAF loop.
  // All three were TIME-GATED — the rAF loop fired whether or not
  // the graph was actually ready to be fitted. The D37ag geometry
  // dump confirmed the failure mode in production: cy.width was 0
  // at forced reveal, meaning the loop ran while cy had no viewport.
  //
  // D37ah replaces the time-gated loop with a READINESS-GATED
  // two-mode lifecycle. Six discrete states (per per-mount
  // docblock below):
  //
  //   awaiting_container → stabilising → revealed                  (clean)
  //                                    → forced_reveal_cy_zero
  //                                    → forced_reveal_fit_failed
  //                                    → (safety cap; see rule)
  //   awaiting_container → blocked_zero_container                  (cap)
  //
  // In 'stabilising' state, the rAF loop checks 9 fit-input gates
  // each frame and only attempts a fit when ALL gates pass. Failing
  // gates emit engine_readiness_gate_failed. A failed fit pipeline
  // emits engine_readiness_fit_failed. Reveal occurs after
  // INITIAL_FIT_STABLE_FRAMES consecutive stable post-fit snapshots.
  //
  // The single external safety-cap setTimeout (INITIAL_FIT_SAFETY_MS)
  // owns the three-way SAFETY-CAP RULE — fan-out at firing time by
  // state.
  //
  // After reveal, a single guarded post-reveal correction may fire
  // inside POST_REVEAL_STABILISATION_MS if the FIRST RO tick after
  // reveal carries a materially different snapshot — catching late
  // layout settling that landed just after stability detection. The
  // correction is disabled once the user interacts with the camera
  // or the grace window expires; D37u steady-state then takes over.
  // Eligibility includes 'revealed' AND 'forced_reveal_cy_zero'.
  var DIAG_ENGINE_INITIAL_STABILISATION_STARTED       = 'engine_initial_stabilisation_started';
  var DIAG_ENGINE_INITIAL_STABILISATION_SAMPLE        = 'engine_initial_stabilisation_sample';
  var DIAG_ENGINE_INITIAL_STABILISATION_STABLE        = 'engine_initial_stabilisation_stable';
  var DIAG_ENGINE_INITIAL_FIT_APPLIED                 = 'engine_initial_fit_applied';
  var DIAG_ENGINE_INITIAL_REVEAL                      = 'engine_initial_reveal';
  // D37ah-readiness-gated-initial-fit — Forced-reveal is now split
  // into two distinct codes so dev tools can distinguish the two
  // root causes (cy.width/height never settled vs. cy was sized but
  // fit pipeline never reported success). The D37af unified
  // DIAG_ENGINE_INITIAL_REVEAL_FORCED has been removed.
  var DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO       = 'engine_initial_reveal_forced_cy_zero';
  var DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED    = 'engine_initial_reveal_forced_fit_failed';
  var DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT       = 'engine_post_reveal_stabilisation_fit';
  // D37ag-diagnostic-geometry-dump — Captures the exact geometry
  // inputs and the post-fit camera state at every fit-relevant
  // moment so initial-mount fit and manual Fit can be diffed in dev
  // tools. The dump is emitted AFTER the fit at each site so the
  // post-fit pan/zoom reflect that fit's result. The dump does not
  // change behaviour; it is observational only.
  var DIAG_ENGINE_GEOMETRY_DUMP                       = 'engine_geometry_dump';
  // D37ah-readiness-gated-initial-fit — Two-mode lifecycle
  // diagnostics. The initial render lifecycle now distinguishes:
  //   • 'awaiting_container' — the container has zero size; the
  //     engine waits for the ResizeObserver to fire with positive
  //     dimensions before kicking off the readiness loop.
  //   • 'stabilising' — the readiness loop checks 9 fit-input gates
  //     each rAF. Failing gates emit engine_readiness_gate_failed.
  //     A failed fit pipeline emits engine_readiness_fit_failed.
  //   • 'blocked_zero_container' — terminal failure mode reached
  //     when the safety cap fires while still 'awaiting_container'
  //     (the container never became sized).
  // State transitions emit engine_initial_lifecycle_state_changed
  // and a co-located geometry dump with trigger='state_transition'.
  var DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED     = 'engine_initial_lifecycle_state_changed';
  var DIAG_ENGINE_READINESS_GATE_FAILED               = 'engine_readiness_gate_failed';
  var DIAG_ENGINE_READINESS_FIT_FAILED                = 'engine_readiness_fit_failed';
  var DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED     = 'engine_initial_mount_container_unsized';
  var DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED     = 'engine_initial_mount_container_blocked';

  // D37ah constants (retained verbatim from D37af; the readiness-
  // gated loop reuses the stability counter + epsilon). Tunable, but
  // the defaults below have specific rationale:
  //
  //   INITIAL_FIT_STABLE_FRAMES = 4 — at 60 fps this is ~66 ms of
  //     stability, enough to outlast a single browser layout pass
  //     while staying imperceptible.
  //   INITIAL_FIT_STABILITY_EPSILON_PX = 0.5 — matches the sub-pixel
  //     noise filter used elsewhere (DIM_PROPAGATION_THRESHOLD,
  //     RECT_STABILITY_THRESHOLD_PX) so a fractional reflow doesn't
  //     reset the stability counter.
  //   INITIAL_FIT_SAFETY_MS = 2000 — the safety-cap budget applied
  //     by the single external setTimeout. Three-way fan-out at
  //     firing time (see SAFETY-CAP RULE above).
  //   POST_REVEAL_STABILISATION_MS = 1000 — after reveal, one RO
  //     tick within this window may trigger ONE corrective fit. Any
  //     RO tick after this window is camera-preserving per D37u.
  //   POST_REVEAL_MAX_CORRECTIONS = 1 — exactly one one-shot
  //     correction per mount.
  var INITIAL_FIT_STABLE_FRAMES        = 4;
  var INITIAL_FIT_STABILITY_EPSILON_PX = 0.5;
  var INITIAL_FIT_SAFETY_MS            = 2000;
  var POST_REVEAL_STABILISATION_MS     = 1000;
  var POST_REVEAL_MAX_CORRECTIONS      = 1;

  // ── Dimension propagation constant ─────────────────────────────────
  //
  // DIM_PROPAGATION_THRESHOLD — minimum delta (in CSS px) between the
  // overlay's measured card footprint and the cy node's `data.width`/
  // `data.height` before the engine writes the new value back to cy.
  // The threshold filters sub-pixel measurement noise that browsers
  // produce from fractional getBoundingClientRect readings (zoom,
  // subpixel layout, font kerning). Documented in the
  // `_propagateDimensions` PROPAGATION CONTRACT comment block below.
  var DIM_PROPAGATION_THRESHOLD = 0.5;

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _isFn(v) { return typeof v === 'function'; }

  function _str(v) { return v == null ? '' : String(v); }

  function _arr(v) { return Array.isArray(v) ? v : []; }

  function _classString(v) {
    if (Array.isArray(v)) return v.filter(Boolean).join(' ');
    return _str(v);
  }

  function _shallowAssign(a, b) {
    if (!a) a = {};
    if (b && typeof b === 'object') {
      for (var k in b) {
        if (Object.prototype.hasOwnProperty.call(b, k)) a[k] = b[k];
      }
    }
    return a;
  }

  // ── Fit-math bounding-box options ────────────────────────────────
  //
  // D37x-engine-node-geometry-contract — Fit math must use declared
  // node BODIES, not rendered label / overlay / outline overflow.
  //
  // Cytoscape's `collection.boundingBox()` defaults include labels,
  // main labels, source labels, target labels, overlays, underlays,
  // and outlines (D37t §2.5 finding from the local bundled library).
  // When the engine fits to the platform-supplied usable rect or
  // safe-area padding, the bounding box it consumes must reflect
  // ONLY the declared node footprint geometry — otherwise long
  // labels, wrap-induced label height, selection overlays, or focus
  // outlines can inflate the bbox and produce a smaller (over-padded)
  // zoom for the same data.
  //
  // This single options object is reused by every engine fit path so
  // the platform behaviour is one source of truth.
  var FIT_BBOX_OPTS = {
    includeNodes:        true,
    includeEdges:        true,
    includeLabels:       false,
    includeMainLabels:   false,
    includeSourceLabels: false,
    includeTargetLabels: false,
    includeOverlays:     false,
    includeUnderlays:    false,
    includeOutlines:     false,
  };

  // ── Fit padding resolution ────────────────────────────────────────
  //
  // _resolveFitPadding normalises one of three input shapes into a
  // per-side `{top, right, bottom, left}` object:
  //
  //   1. scalar number (back-compat) — applied uniformly to all four
  //      sides. The pre-tranche public API was `cy.fit(undefined, 24)`
  //      via `handle.fit()`; callers passing a scalar continue to work.
  //   2. per-side `{top, right, bottom, left}` object — used verbatim
  //      with `DEFAULT_FIT_PADDING` floor on missing sides.
  //   3. `null` / undefined — pull safe-area insets from
  //      `getSafeArea()` (a mount option supplied by the lens) and
  //      compose with `DEFAULT_FIT_PADDING` as a floor (per side).
  //      This is the strategic path: the engine consumes GraphViewport's
  //      live chrome measurement and produces per-side insets that
  //      keep the graph clear of every visible chrome surface.
  //
  // The composition rule for case 3 is `max(DEFAULT_FIT_PADDING, sa.<side>)`
  // per side — i.e. the engine never reserves less than the default,
  // and reserves more when the host's chrome demands it.
  function _resolveFitPadding(input, getSafeArea) {
    if (typeof input === 'number' && isFinite(input) && input >= 0) {
      return { top: input, right: input, bottom: input, left: input };
    }
    if (_isPlainObject(input)) {
      return {
        top:    (typeof input.top    === 'number' && isFinite(input.top))    ? input.top    : DEFAULT_FIT_PADDING,
        right:  (typeof input.right  === 'number' && isFinite(input.right))  ? input.right  : DEFAULT_FIT_PADDING,
        bottom: (typeof input.bottom === 'number' && isFinite(input.bottom)) ? input.bottom : DEFAULT_FIT_PADDING,
        left:   (typeof input.left   === 'number' && isFinite(input.left))   ? input.left   : DEFAULT_FIT_PADDING,
      };
    }
    if (_isFn(getSafeArea)) {
      var sa = null;
      try { sa = getSafeArea(); } catch (_) { sa = null; }
      if (_isPlainObject(sa)) {
        return {
          top:    Math.max(DEFAULT_FIT_PADDING, (typeof sa.top    === 'number') ? sa.top    : 0),
          right:  Math.max(DEFAULT_FIT_PADDING, (typeof sa.right  === 'number') ? sa.right  : 0),
          bottom: Math.max(DEFAULT_FIT_PADDING, (typeof sa.bottom === 'number') ? sa.bottom : 0),
          left:   Math.max(DEFAULT_FIT_PADDING, (typeof sa.left   === 'number') ? sa.left   : 0),
        };
      }
    }
    return { top: DEFAULT_FIT_PADDING, right: DEFAULT_FIT_PADDING, bottom: DEFAULT_FIT_PADDING, left: DEFAULT_FIT_PADDING };
  }

  // ── Safe-area-aware fit ───────────────────────────────────────────
  //
  // _fitWithSafeArea computes a `cy.viewport({zoom, pan})` transform
  // that fits the graph's visible elements into the cy container's
  // canvas MINUS per-side safe-area insets. The algorithm mirrors
  // Authority's `_fitToAvailableCanvas` (authority-cytoscape-poc.js:
  // 898-982) and includes the degenerate-viewport guard that prevents
  // negative or impossibly-small visible regions.
  //
  // Cytoscape's built-in `cy.fit(eles, padding)` accepts only a
  // SCALAR padding (forced uniform on every side); this helper exists
  // because per-side awareness is required for chrome-aware fit
  // (different chrome reaches each viewport edge differently — left
  // rail, right drawer, bottom tray, top toolbar).
  //
  // Algorithm:
  //   1. Read the elements' bounding box and the cy canvas dimensions.
  //   2. Apply degenerate-viewport guard: if insets would collapse the
  //      visible region below FIT_MIN_VISIBLE_PX, scale the relevant
  //      side insets back proportionally so the graph stays visible.
  //   3. Compute zoom = min(visibleW/bb.w, visibleH/bb.h), clamped to
  //      [FIT_ZOOM_MIN, FIT_ZOOM_MAX] and cy's own min/max.
  //   4. Compute pan so the bb centre lands at the centre of the
  //      visible region (inset.left + visibleW/2, inset.top + visibleH/2).
  //   5. Apply via `cy.viewport({zoom, pan})` — atomic so cy fires
  //      one render event, not two.
  function _fitWithSafeArea(cy, padding) {
    if (!cy) return;
    var eles;
    try { eles = cy.elements(':visible'); }
    catch (_) {
      try { eles = cy.elements(); }
      catch (__) { return; }
    }
    if (!eles || eles.length === 0) return;
    var bb;
    // D37x-engine-node-geometry-contract — pass FIT_BBOX_OPTS so the
    // bbox reflects declared node bodies only. Labels, overlays,
    // underlays, and outlines are excluded.
    try { bb = eles.boundingBox(FIT_BBOX_OPTS); } catch (_) { return; }
    if (!bb || !(bb.w > 0) || !(bb.h > 0)) return;

    var cw = _isFn(cy.width)  ? cy.width()  : 0;
    var ch = _isFn(cy.height) ? cy.height() : 0;
    if (!(cw > 0) || !(ch > 0)) return;

    var L = padding.left, R = padding.right, T = padding.top, B = padding.bottom;

    // ── Degenerate-viewport guard ──
    //
    // If the horizontal insets would collapse the visible width below
    // FIT_MIN_VISIBLE_PX, scale L and R back proportionally so the
    // graph still gets a usable visible band. Same for vertical.
    if (cw - L - R < FIT_MIN_VISIBLE_PX) {
      var hSlack  = cw - FIT_MIN_VISIBLE_PX;
      var hWeight = L + R;
      if (hSlack > 0 && hWeight > 0) {
        L = Math.max(DEFAULT_FIT_PADDING, L * hSlack / hWeight);
        R = Math.max(DEFAULT_FIT_PADDING, R * hSlack / hWeight);
      } else {
        L = DEFAULT_FIT_PADDING;
        R = DEFAULT_FIT_PADDING;
      }
    }
    if (ch - T - B < FIT_MIN_VISIBLE_PX) {
      var vSlack  = ch - FIT_MIN_VISIBLE_PX;
      var vWeight = T + B;
      if (vSlack > 0 && vWeight > 0) {
        T = Math.max(DEFAULT_FIT_PADDING, T * vSlack / vWeight);
        B = Math.max(DEFAULT_FIT_PADDING, B * vSlack / vWeight);
      } else {
        T = DEFAULT_FIT_PADDING;
        B = DEFAULT_FIT_PADDING;
      }
    }

    var vw = Math.max(FIT_MIN_VISIBLE_PX, cw - L - R);
    var vh = Math.max(FIT_MIN_VISIBLE_PX, ch - T - B);

    var z = Math.min(vw / bb.w, vh / bb.h);
    var cyMax = _isFn(cy.maxZoom) ? cy.maxZoom() : Infinity;
    var cyMin = _isFn(cy.minZoom) ? cy.minZoom() : 0;
    z = Math.min(z, isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);
    z = Math.max(z, cyMin || 0, FIT_ZOOM_MIN);

    var rcx = L + vw / 2;
    var rcy = T + vh / 2;
    var gx  = bb.x1 + bb.w / 2;
    var gy  = bb.y1 + bb.h / 2;

    try {
      cy.viewport({ zoom: z, pan: { x: rcx - gx * z, y: rcy - gy * z } });
    } catch (_) { /* swallow — cy rejects invalid viewports silently */ }
  }

  // ── Dimension propagation ─────────────────────────────────────────
  //
  // ── PROPAGATION CONTRACT (load-bearing) ──
  //
  // _propagateDimensions writes the overlay-measured visible card
  // footprint back to the corresponding cy node's `data.width` /
  // `data.height`. The cy 'node' style at `_buildBaseStyle` binds
  // these via `'width': 'data(width)'` / `'height': 'data(height)'`,
  // so the data write resizes the cy node's bounding box, and cy
  // re-clips incident edges to the new outline on the next render.
  // This is how the engine keeps connector endpoints aligned to the
  // VISIBLE card boundary regardless of the lens template's CSS
  // dimensions or dynamic content (selection border, hover padding,
  // content updates).
  //
  // Loop shape (must converge):
  //
  //   overlay._measureCard → onMeasure(key, w, h)
  //     ↓
  //   engine._propagateDimensions → cy.node.data({width, height})
  //     ↓
  //   cy 'data' event → cy re-styles node → cy 'render' event
  //     ↓
  //   overlay SYNC_EVENTS handler → overlay._sync → overlay._syncCard
  //     ↓
  //   overlay reads cached entry.measuredWidth/Height (NOT re-measured
  //   here — re-measure only happens via the per-card ResizeObserver
  //   when the visible card's content/layout actually changes)
  //
  // The loop terminates because the overlay's `_sync` does NOT call
  // `_measureCard`; it only reads cached dimensions. So a cy data
  // write doesn't re-enter the propagation path.
  //
  // Guard: a 0.5-px threshold (DIM_PROPAGATION_THRESHOLD) filters
  // sub-pixel measurement noise. Browsers sometimes report fractional
  // getBoundingClientRect values that fluctuate without semantic size
  // changes (zoom level, subpixel layout, font kerning). The threshold
  // ensures cy data is only written when the visible card has
  // CHANGED size by a perceptible amount. Without the guard, a steady
  // stream of sub-pixel ResizeObserver callbacks would trigger
  // unnecessary cy data writes and re-renders.
  //
  // Events that MUST NOT re-trigger propagation:
  //
  //   • cy 'position'                — node moved, not resized.
  //   • cy 'pan', 'zoom', 'render'   — viewport changed, not size.
  //   • cy 'select', 'unselect',     — state-class application is
  //     'mouseover', 'mouseout'        purely CSS; if it changes the
  //                                    visible card's measured size,
  //                                    the per-card ResizeObserver
  //                                    fires legitimately and propagation
  //                                    re-enters with new dimensions —
  //                                    but never directly from these
  //                                    events.
  //
  // The overlay's onMeasure callback is the SOLE entry point that
  // writes cy.data('width'/'height') after the initial mount. The
  // initial values come from the lens's canonical data shape
  // (`data.width`, `data.height` on each node); subsequent updates
  // flow through this propagation function only.
  function _propagateDimensions(cy, key, w, h) {
    if (!cy || !key) return;
    if (!(typeof w === 'number' && isFinite(w) && w > 0)) return;
    if (!(typeof h === 'number' && isFinite(h) && h > 0)) return;
    var n;
    try { n = cy.getElementById(_str(key)); }
    catch (_) { return; }
    if (!n || !n.length) return;
    var data;
    try { data = n.data() || {}; }
    catch (_) { return; }
    var curW = (typeof data.width  === 'number' && isFinite(data.width))  ? data.width  : NaN;
    var curH = (typeof data.height === 'number' && isFinite(data.height)) ? data.height : NaN;
    var wDelta = isFinite(curW) ? Math.abs(w - curW) : Infinity;
    var hDelta = isFinite(curH) ? Math.abs(h - curH) : Infinity;
    if (wDelta < DIM_PROPAGATION_THRESHOLD && hDelta < DIM_PROPAGATION_THRESHOLD) return;
    try { n.data({ width: w, height: h }); } catch (_) { /* swallow */ }
  }

  // ── Strategic fit-to-usable-rect ─────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — STRATEGIC FIT-ENVELOPE CONTRACT.
  //
  // `_fitToUsableRect(cy, usableRect, diagnosticsSink)` fits the
  // graph's bounding box into the platform-supplied usable rectangle.
  // The usable rectangle is the actual area inside the graph viewport
  // that's NOT covered by MIDAS chrome (right drawer, bottom tray,
  // camera cluster, etc.). The engine does NOT compute chrome offsets
  // itself — the platform's `GraphViewport.getUsableGraphRect()` is
  // the single source of truth.
  //
  // The usable rect's `x` / `y` are viewport-relative (NOT document-
  // relative); the cy canvas IS the viewport's renderer slot at
  // `position: absolute; inset: 0` inside the engine's mount
  // container, so usableRect.x / .y are directly the per-side insets
  // the engine must reserve.
  //
  // Algorithm:
  //   1. Read cy elements' bounding box. If empty, emit
  //      `fit_bounds_empty` and return.
  //   2. If usableRect is zero or below FIT_MIN_VISIBLE_PX in either
  //      dimension, emit `usable_rect_empty` or
  //      `usable_rect_smaller_than_minimum` and fall back to fitting
  //      against the full cy container (avoid hiding the graph).
  //   3. Compute zoom = min(usableRect.width / bb.w,
  //                         usableRect.height / bb.h).
  //   4. Clamp zoom against `[FIT_ZOOM_MIN, FIT_ZOOM_MAX]` and cy's
  //      own min/max. Emit `fit_zoom_clamped_min` /
  //      `fit_zoom_clamped_max` if the clamp engaged.
  //   5. Compute pan so the bb's centre lands at the centre of the
  //      usable rect's footprint in cy-canvas coordinates.
  //   6. Apply atomically via `cy.viewport({zoom, pan})`.
  //
  // This is the platform's strategic fit. The legacy `_fitWithSafeArea`
  // (per-side padding) remains as a fallback for consumers that
  // supply `getSafeArea` but not `getUsableGraphRect`.
  function _fitToUsableRect(cy, usableRect, emitDiag) {
    if (!cy) return false;
    var eles;
    try { eles = cy.elements(':visible'); }
    catch (_) {
      try { eles = cy.elements(); }
      catch (__) { return false; }
    }
    if (!eles || eles.length === 0) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_BOUNDS_EMPTY);
      return false;
    }
    var bb;
    // D37x-engine-node-geometry-contract — pass FIT_BBOX_OPTS so the
    // bbox reflects declared node bodies only. Labels, overlays,
    // underlays, and outlines are excluded.
    try { bb = eles.boundingBox(FIT_BBOX_OPTS); } catch (_) { return false; }
    if (!bb || !(bb.w > 0) || !(bb.h > 0)) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_BOUNDS_EMPTY);
      return false;
    }

    var cw = _isFn(cy.width)  ? cy.width()  : 0;
    var ch = _isFn(cy.height) ? cy.height() : 0;
    if (!(cw > 0) || !(ch > 0)) return false;

    // Validate the usable rect. If unusable, the caller falls back
    // to `_fitWithSafeArea` with a uniform DEFAULT_FIT_PADDING.
    if (!_isPlainObject(usableRect)) return false;
    var uw = usableRect.width, uh = usableRect.height;
    if (!(uw > 0) || !(uh > 0)) {
      if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_EMPTY);
      return false;
    }
    if (uw < FIT_MIN_VISIBLE_PX || uh < FIT_MIN_VISIBLE_PX) {
      if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_SMALLER_THAN_MIN);
      // Don't return false — try to fit anyway against the degraded
      // rect, but emit the diagnostic so consumers see the condition.
    }

    var ux = (typeof usableRect.x === 'number' && isFinite(usableRect.x)) ? usableRect.x : 0;
    var uy = (typeof usableRect.y === 'number' && isFinite(usableRect.y)) ? usableRect.y : 0;

    var zRaw = Math.min(uw / bb.w, uh / bb.h);
    var cyMax = _isFn(cy.maxZoom) ? cy.maxZoom() : Infinity;
    var cyMin = _isFn(cy.minZoom) ? cy.minZoom() : 0;
    var zMaxClamp = Math.min(isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);
    var zMinClamp = Math.max(cyMin || 0, FIT_ZOOM_MIN);
    var z = Math.min(zRaw, zMaxClamp);
    if (z < zMinClamp) {
      z = zMinClamp;
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_ZOOM_CLAMPED_MIN);
    } else if (zRaw > zMaxClamp) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_ZOOM_CLAMPED_MAX);
    }

    // The cy container fills the engine's mount inset:0; the usable
    // rect's (x, y) are viewport-relative — so the centre of the
    // usable rect within the cy canvas is (ux + uw/2, uy + uh/2).
    var rcx = ux + uw / 2;
    var rcy = uy + uh / 2;
    var gx  = bb.x1 + bb.w / 2;
    var gy  = bb.y1 + bb.h / 2;

    try {
      cy.viewport({ zoom: z, pan: { x: rcx - gx * z, y: rcy - gy * z } });
      return true;
    } catch (_) { return false; }
  }

  // ── Engine-level non-overlap validation ───────────────────────────
  //
  // D37s-context-geometry-2-impl — Global non-overlap invariant.
  //
  // ── PLATFORM CONTRACT ──
  //
  //   graphStage computes non-overlapping positions for stage-managed
  //   layouts (given accurate footprints).
  //   graphCytoscapeEngine VALIDATES non-overlap for every Cytoscape-
  //   backed graph it mounts, using MEASURED runtime card dimensions.
  //   Lens-specific placement algorithms (e.g. Authority's
  //   _computePresetPositions, pending B''-Authority migration) MUST
  //   satisfy this same engine-validated invariant once they consume
  //   the engine.
  //
  // The engine does NOT silently mutate supplied positions. It
  // validates and reports overlap; it does not become a hidden
  // layout planner. This is the global rule enforcement point: a
  // future build may add explicit, opt-in collision resolution at
  // the engine level, but this build's contract is "engine validates;
  // lenses comply."
  //
  // Validation inputs:
  //   • Cy node MODEL position (lens-supplied, unmutated by engine).
  //     Reads via `cy.nodes().forEach(n => n.position())`.
  //   • Measured card dimensions (overlay-measured, propagated via
  //     `_propagateDimensions` to cy.data(width/height)). Reads via
  //     `n.data('width')` / `n.data('height')`.
  //   • Centre-based geometry: a node's bbox is
  //     `[pos.x - w/2, pos.y - h/2, pos.x + w/2, pos.y + h/2]`.
  //
  // Diagnostics emitted:
  //   • console.warn under tag `[graph-cytoscape-engine]` for each
  //     overlap pair, ONCE per debounce window per pair (de-dup'd by
  //     `<idA>|<idB>` key).
  //   • Stored on the handle via `handle.getDiagnostics()` so tests
  //     and dev tools can inspect without subscribing to console.
  //
  // The validator is bounded:
  //   • Debounced via `OVERLAP_VALIDATION_DEBOUNCE_MS` (250 ms).
  //   • Diagnostics dedup'd via a per-pair seen-set (reset on each
  //     successful no-overlap validation, so a re-emergence after a
  //     resolution re-fires the warning).
  function _validateNoOverlap(cy) {
    if (!cy) return { overlaps: [] };
    var nodes;
    try { nodes = cy.nodes(); } catch (_) { return { overlaps: [] }; }
    if (!nodes || typeof nodes.length !== 'number' || nodes.length < 2) {
      return { overlaps: [] };
    }
    // Materialise per-node bbox in MODEL space.
    var boxes = [];
    for (var i = 0; i < nodes.length; i++) {
      var n;
      try { n = nodes[i]; } catch (_) { continue; }
      if (!n || !n.length) continue;
      var pos;
      try { pos = n.position(); } catch (_) { pos = null; }
      if (!pos) continue;
      var data;
      try { data = n.data() || {}; } catch (_) { data = {}; }
      var w = (typeof data.width  === 'number' && isFinite(data.width)  && data.width  > 0) ? data.width  : 0;
      var h = (typeof data.height === 'number' && isFinite(data.height) && data.height > 0) ? data.height : 0;
      if (!(w > 0) || !(h > 0)) continue;
      boxes.push({
        id: n.id(),
        x0: pos.x - w / 2,
        y0: pos.y - h / 2,
        x1: pos.x + w / 2,
        y1: pos.y + h / 2,
      });
    }
    // Pairwise overlap check. O(N²) is fine for typical graph sizes
    // (10-50 nodes). For very large graphs a spatial-index variant
    // would land in a later tranche.
    var overlaps = [];
    for (var a = 0; a < boxes.length; a++) {
      for (var b = a + 1; b < boxes.length; b++) {
        var ba = boxes[a], bb = boxes[b];
        if (ba.x0 < bb.x1 && bb.x0 < ba.x1 && ba.y0 < bb.y1 && bb.y0 < ba.y1) {
          overlaps.push({ cardA: ba.id, cardB: bb.id });
        }
      }
    }
    return { overlaps: overlaps };
  }

  // ── Canonical data → Cytoscape elements ───────────────────────────

  // _toCyElements converts the lens-supplied canonical data shape
  // into Cytoscape's elements-array format. Per-node `data.id` is
  // taken from `node.id`; per-edge `data.source` / `data.target`
  // reference node ids; position is taken verbatim from the lens
  // (which is responsible for preset placement). Per-element
  // `classes` is normalised from array-or-string into the cy
  // space-separated string form. Lens-supplied `data` is merged in
  // so lens-specific fields (kind, visualClass, dashPattern, etc.)
  // are reachable via `cy.data()` for the lens's selection /
  // template logic.
  function _toCyElements(data) {
    var out = [];
    if (!_isPlainObject(data)) return out;
    var nodes = _arr(data.nodes);
    var edges = _arr(data.edges);
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (!_isPlainObject(n) || n.id == null) continue;
      var nodeData = _shallowAssign({ id: _str(n.id), kind: _str(n.kind) }, _isPlainObject(n.data) ? n.data : null);
      out.push({
        group:     'nodes',
        data:      nodeData,
        position:  _isPlainObject(n.position) ? n.position : { x: 0, y: 0 },
        classes:   _classString(n.classes),
        selectable: true,
        grabbable:  false,
      });
    }
    for (var j = 0; j < edges.length; j++) {
      var e = edges[j];
      if (!_isPlainObject(e) || e.id == null || e.source == null || e.target == null) continue;
      var edgeData = _shallowAssign({
        id:          _str(e.id),
        source:      _str(e.source),
        target:      _str(e.target),
        kind:        _str(e.kind),
        visualClass: _str(e.visualClass),
      }, _isPlainObject(e.data) ? e.data : null);
      out.push({
        group:      'edges',
        data:       edgeData,
        classes:    _classString(e.classes),
        selectable: false,
      });
    }
    return out;
  }

  // ── Base style ─────────────────────────────────────────────────────

  // _buildBaseStyle returns the engine's invariant cy style array.
  // The base style declares:
  //
  //   • a transparent node (no fill, no border, no label) — the
  //     overlay is always the visible card layer;
  //   • a `node:selected` rule with `overlay-opacity: 0` so
  //     Cytoscape's default selection halo never paints over the
  //     overlay card;
  //   • a base edge rule with a sensible default colour, width,
  //     and `curve-style: bezier`. Lenses supply per-class edge
  //     styling (visual classes, dash patterns) via
  //     `options.nodeStyleOverride`, which is appended to this
  //     base.
  //
  // The engine's transparent-node policy is invariant. A lens that
  // wants to display visible cy nodes (e.g. a debug mode) MUST
  // override at the engine level — not via `nodeStyleOverride`, to
  // preserve the architectural commitment that overlays are the
  // visible card surface.
  //
  // ── D37x-engine-node-geometry-contract ──
  //
  // The base style locks the engine's NODE GEOMETRY CONTRACT:
  //
  //   • `width:  data(width)`  — declared per-node body width.
  //   • `height: data(height)` — declared per-node body height.
  //   • `label:  ''`           — no native Cytoscape label by default.
  //
  // Rules the contract enforces (asserted by source-contract tests):
  //
  //   1. data.width and data.height are REQUIRED on every node. The
  //      lens supplies them through canonical data; the engine
  //      validates positive finite numbers at element-build time
  //      and emits `node_geometry_invalid_dimensions` when violated.
  //   2. data.label is OPTIONAL. Absence → `label: ''` (the engine
  //      default) → cy renders no native label. Lenses that paint
  //      the visible card via an HTML overlay (e.g. Authority's
  //      html-card theme) MUST leave data.label absent.
  //   3. Labels never define or enlarge node geometry. The base
  //      style binds dimensions to `data(width)` / `data(height)`
  //      and NEVER to the `'label'` enum that Cytoscape supports for
  //      node-sizes-from-label. That enum is strictly forbidden for
  //      production lenses (a source-contract test guards it).
  //   4. Labels never participate in fit math. The engine's fit
  //      pipeline calls `boundingBox(FIT_BBOX_OPTS)` with
  //      `includeLabels: false` and the related label / overlay /
  //      outline flags off so the fit bbox reflects only the
  //      declared bodies.
  //   5. Lenses that DO opt into native labels MUST pre-truncate the
  //      label via `graphNativeLabels.makeNativeNodeLabel(...)`
  //      before writing it to `data.label`. The helper bounds the
  //      label to the declared body at the engine's native-label
  //      font / line / padding constants.
  function _buildBaseStyle() {
    return [
      {
        selector: 'node',
        style: {
          'width':              'data(width)',
          'height':             'data(height)',
          'background-color':   'rgba(0,0,0,0)',
          'background-opacity': 0,
          'border-width':       0,
          'label':              '',
          'shape':              'round-rectangle',
        },
      },
      {
        selector: 'node:selected',
        style: { 'overlay-opacity': 0 },
      },
      {
        selector: 'edge',
        style: {
          'width':              1.2,
          'curve-style':        'bezier',
          'line-color':         '#9aa4b2',
          'target-arrow-shape': 'none',
          'source-arrow-shape': 'none',
          'opacity':            0.78,
        },
      },
    ];
  }

  // ── Mount ──────────────────────────────────────────────────────────

  function mount(mountEl, options) {
    if (!mountEl || typeof mountEl.appendChild !== 'function') {
      throw new Error('graphCytoscapeEngine.mount: mountEl must be a DOM element');
    }
    if (typeof window.cytoscape !== 'function') {
      throw new Error('graphCytoscapeEngine.mount: window.cytoscape is not defined; check vendor script-tag order');
    }
    var opts = _isPlainObject(options) ? options : {};
    var lensId = _str(opts.lensId);
    if (!lensId) {
      throw new Error('graphCytoscapeEngine.mount: options.lensId is required');
    }
    if (!_isPlainObject(opts.data)) {
      throw new Error('graphCytoscapeEngine.mount: options.data is required (canonical {nodes, edges})');
    }
    if (!_isPlainObject(opts.template) || !_isFn(opts.template.create)) {
      throw new Error('graphCytoscapeEngine.mount: options.template.create(node, ctx) is required');
    }
    if (!_isFn(opts.keyForNode)) {
      throw new Error('graphCytoscapeEngine.mount: options.keyForNode(node) is required');
    }
    if (!_isFn(opts.selectionAdapter)) {
      throw new Error('graphCytoscapeEngine.mount: options.selectionAdapter(cyEvent, handle) is required');
    }
    if (!_isFn(opts.cameraAdapter)) {
      throw new Error('graphCytoscapeEngine.mount: options.cameraAdapter(handle) is required');
    }

    // ── Engine container ──
    //
    // The engine creates a single positioning context that hosts BOTH
    // the cy canvas mount AND the overlay layer as inset:0 siblings.
    // This guarantees both surfaces render in the same coordinate
    // frame — the core mechanism the engine extraction is intended to
    // unify. The container fills its parent (`mountEl` must be
    // pre-sized by the lens). `overflow: hidden` clips overflowing
    // cards / canvases to the container; `position: relative` makes
    // the container the containing block for the absolute-positioned
    // siblings inside.
    var container = document.createElement('div');
    container.className = CONTAINER_CLASS;
    container.setAttribute('data-lens', lensId);
    container.style.position = 'relative';
    container.style.width    = '100%';
    container.style.height   = '100%';
    container.style.overflow = 'hidden';
    // D37ab-graph-engine-initial-fit-before-reveal — Mount the engine
    // surface INVISIBLE. The graph is held back until the first
    // successful safe-area-aware fit applies (or a bounded safety
    // timer force-reveals). `visibility: hidden` is the chosen
    // mechanism because — unlike `display: none` — it preserves the
    // container's box and lets Cytoscape's mount-time measurement
    // (cy.width / cy.height / cy.elements().boundingBox(...)) succeed.
    // The `data-initial-fit` attribute is the test-pinnable lifecycle
    // signal; downstream consumers may also key CSS off it if they
    // want a fade-in (none required by this tranche).
    container.style.visibility = 'hidden';
    container.setAttribute('data-initial-fit', 'pending');
    mountEl.appendChild(container);

    // ── Cy mount ──
    //
    // Cy mounts in its own DIV inside the engine container.
    // `position: absolute; inset: 0` puts the cy mount in the same
    // coordinate frame as the overlay (which is also `inset: 0`).
    // Cy is responsible for sizing its internal canvases to match
    // this DIV's clientWidth/clientHeight.
    var cyMount = document.createElement('div');
    cyMount.className = CY_MOUNT_CLASS;
    cyMount.style.position = 'absolute';
    cyMount.style.left     = '0';
    cyMount.style.top      = '0';
    cyMount.style.right    = '0';
    cyMount.style.bottom   = '0';
    container.appendChild(cyMount);

    // ── Build cy elements + style ──

    var cyElements = _toCyElements(opts.data);
    var styleArray = _buildBaseStyle().concat(_arr(opts.nodeStyleOverride));

    // D37x-engine-node-geometry-contract — Pre-scan supplied data
    // for geometry violations. Each node MUST arrive with positive
    // finite `data.width` and `data.height`. We record the result
    // as a closure flag here; the dedup'd diagnostic emit happens
    // once `_recordFitDiagnostic` is defined further down in mount().
    var _nodeGeometryViolated = false;
    if (_isPlainObject(opts.data) && Array.isArray(opts.data.nodes)) {
      for (var _gi = 0; _gi < opts.data.nodes.length; _gi++) {
        var _sn = opts.data.nodes[_gi];
        if (!_isPlainObject(_sn) || _sn.id == null) continue;
        var _snData = _isPlainObject(_sn.data) ? _sn.data : {};
        var _gw = _snData.width, _gh = _snData.height;
        var _gWOk = (typeof _gw === 'number' && isFinite(_gw) && _gw > 0);
        var _gHOk = (typeof _gh === 'number' && isFinite(_gh) && _gh > 0);
        if (!_gWOk || !_gHOk) { _nodeGeometryViolated = true; break; }
      }
    }

    // ── Instantiate Cytoscape ──

    var cy;
    try {
      cy = window.cytoscape({
        container:            cyMount,
        elements:             cyElements,
        style:                styleArray,
        layout:               { name: 'preset', fit: false },
        wheelSensitivity:     0.2,
        boxSelectionEnabled:  false,
        autounselectify:      false,
        userZoomingEnabled:   true,
        userPanningEnabled:   true,
        minZoom:              0.1,
        maxZoom:              4,
      });
    } catch (e) {
      try { container.parentNode.removeChild(container); } catch (_) { /* swallow */ }
      throw e;
    }

    // ── Handle (forward-declared so adapters can capture it) ──

    var ZOOM_STEP = 1.25;
    var ZOOM_MIN  = 0.1;
    var ZOOM_MAX  = 4;
    function _clampZoom(z) {
      if (typeof z !== 'number' || !isFinite(z)) return null;
      if (z < ZOOM_MIN) return ZOOM_MIN;
      if (z > ZOOM_MAX) return ZOOM_MAX;
      return z;
    }

    var _destroyed = false;
    var overlayHandle = null;
    var resizeObs = null;
    var _selectionSetClearRequester = null;

    // D37u-cytoscape-resize-policy-impl — No per-mount resize state.
    //
    // The resize lifecycle holds no state: every tick calls
    // `cy.resize()` and returns. No timers, no rect cache, no
    // re-entry guards. Camera preservation is the default policy and
    // is enforced by NOT calling fit/viewport on the passive path.

    // D37ah-readiness-gated-initial-fit — Per-mount initial render
    // lifecycle state machine (clean break from D37af time-gated
    // stabilisation). Six discrete states, gated by READINESS rather
    // than by elapsed time:
    //
    //   'awaiting_container'      — the container has zero size at
    //                               mount. The readiness loop is NOT
    //                               running; the engine waits for a
    //                               ResizeObserver tick to deliver
    //                               positive container dimensions
    //                               before transitioning to
    //                               'stabilising'.
    //   'stabilising'             — the rAF readiness loop runs each
    //                               frame: checks 9 fit-input gates,
    //                               runs the canonical fit pipeline
    //                               when all gates pass, and reveals
    //                               after INITIAL_FIT_STABLE_FRAMES
    //                               consecutive stable snapshots.
    //   'revealed'                — readiness gates all passed and
    //                               snapshots stabilised; visibility
    //                               restored cleanly.
    //   'forced_reveal_cy_zero'   — safety cap fired while still
    //                               'stabilising' AND cy.width or
    //                               cy.height was 0. The container
    //                               became visible but the cy mount
    //                               never reported a non-zero
    //                               viewport.
    //   'forced_reveal_fit_failed' — safety cap fired while still
    //                                'stabilising', cy was sized, but
    //                                _runFitPipeline never reported
    //                                fitApplied = true within the
    //                                window. Reveal proceeds without
    //                                a guaranteed fit.
    //   'blocked_zero_container'  — safety cap fired while state was
    //                               still 'awaiting_container'. The
    //                               container never became sized; the
    //                               graph stays hidden. Operator
    //                               intervention (parent layout fix)
    //                               required.
    //
    // SAFETY-CAP RULE (three-way) — applied by the single external
    //   safety-cap setTimeout at INITIAL_FIT_SAFETY_MS:
    //
    //     1. state === 'awaiting_container'
    //          → _transitionTo('blocked_zero_container');
    //            emit DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED.
    //          NOTE: this branch does NOT call _revealGraph; the
    //          container stays hidden.
    //     2. state === 'stabilising' AND cy.width/height === 0
    //          → _revealGraph('forced_cy_zero').
    //     3. state === 'stabilising' AND cy.width/height > 0
    //          → _revealGraph('forced_fit_failed') — fit attempts
    //            either ran and reported failure, or never reached
    //            the fit pipeline within the window.
    //
    // POST-REVEAL CORRECTION:
    //   Allowed from BOTH 'revealed' and 'forced_reveal_cy_zero'
    //   states (the latter because a late ResizeObserver tick may
    //   finally deliver positive cy dimensions and a corrective fit
    //   becomes valuable). NOT allowed from 'forced_reveal_fit_failed'
    //   (a fit attempted and failed — re-attempting on the same data
    //   is futile) or 'blocked_zero_container' (no reveal happened).
    //
    // Per-mount state vars:
    //   `_initialFitState`               — current state (one of the
    //                                      six above).
    //   `_revealReason`                  — string reason passed to
    //                                      _revealGraph: one of
    //                                      'clean' | 'forced_cy_zero'
    //                                      | 'forced_fit_failed'.
    //                                      Null until reveal.
    //   `_initialFitRevealTimer`         — the safety-cap setTimeout
    //                                      id; cleared by reveal /
    //                                      destroy.
    //   `_lastFitSnapshot`               — most recent valid snapshot
    //                                      taken during the readiness
    //                                      loop.
    //   `_stableFrameCount`              — consecutive-frame counter;
    //                                      ≥ INITIAL_FIT_STABLE_FRAMES
    //                                      triggers clean reveal.
    //   `_fitPipelineAppliedAtLeastOnce` — true once any rAF tick
    //                                      observed _runFitPipeline
    //                                      returning fitApplied=true.
    //                                      Used by the safety cap to
    //                                      pick between
    //                                      'forced_fit_failed' and
    //                                      (defensively) the same.
    //   `_stabilisationStartTs`          — Date.now() at mount;
    //                                      retained for diagnostic
    //                                      payloads and parity.
    //   `_stabilisationRaf`              — pending rAF / fallback
    //                                      timer id for the readiness
    //                                      loop.
    //   `_revealSnapshot`                — snapshot at reveal time;
    //                                      compared by the post-reveal
    //                                      correction guard.
    //   `_postRevealCorrectionsRemaining` — counts down from
    //                                       POST_REVEAL_MAX_CORRECTIONS.
    //   `_postRevealGraceEndTs`          — Date.now() + grace window;
    //                                      corrections only allowed
    //                                      while now() <= this.
    //   `_userInteracted`                — set true on cy pan/zoom/
    //                                      drag/tap events after
    //                                      reveal; disables corrections.
    //   `_userInteractionGuardBound`     — idempotence flag for the
    //                                      cy event subscription.
    var _initialFitState                 = 'awaiting_container';
    var _revealReason                    = null;
    var _initialFitRevealTimer           = 0;
    var _lastFitSnapshot                 = null;
    var _stableFrameCount                = 0;
    var _fitPipelineAppliedAtLeastOnce   = false;
    var _stabilisationStartTs            = 0;
    var _stabilisationRaf                = 0;
    var _revealSnapshot                  = null;
    var _postRevealCorrectionsRemaining  = POST_REVEAL_MAX_CORRECTIONS;
    var _postRevealGraceEndTs            = 0;
    var _userInteracted                  = false;
    var _userInteractionGuardBound       = false;

    // D37s-context-geometry-2-impl — Engine measurement state.
    //
    // The engine maintains a per-node measurement cache. The overlay's
    // `onMeasure(key, w, h)` callback writes here (in addition to
    // calling `_propagateDimensions`). At each measurement event:
    //   1. `_measurementCache[key] = { width, height }` is updated.
    //   2. The lens-supplied `opts.onMeasurementsChange(measurements)`
    //      (if any) is invoked with the FULL current cache so the
    //      lens can re-compute footprints with a complete picture.
    //      Coalesced via `_measurementChangeRaf` so a burst of
    //      per-card measurement events collapses to one lens call
    //      per frame.
    //   3. A debounced `_validateNoOverlap` pass runs (at most once
    //      per OVERLAP_VALIDATION_DEBOUNCE_MS) and produces
    //      diagnostics. Each unique overlap pair fires its console
    //      warn at most once per validation cycle; the seen-set
    //      resets when a validation finds zero overlaps (so a re-
    //      emergence re-fires).
    var _measurementCache       = {};   // { key: {width, height} }
    var _measurementChangeRaf   = 0;
    var _overlapValidationT     = 0;   // last validation timestamp
    var _overlapValidationTimer = 0;   // pending validation handle
    var _overlapSeen            = {};  // { 'idA|idB': true }
    // Diagnostics surfaced via `handle.getDiagnostics()`. Each entry:
    //   { code: 'card_overlap_detected', cardA, cardB, ts }
    var _engineDiagnostics      = [];
    // D37s-viewport-fit-1-impl — Dedup'd fit-diagnostic codes per
    // mount lifetime. Codes (`usable_rect_empty`, `fit_bounds_empty`,
    // etc.) are emitted at most once per mount unless the underlying
    // condition resolves and re-emerges. Conservative dedup so a
    // chatty consumer doesn't spam the console / diagnostics buffer.
    var _fitDiagnosticsSeen     = {};
    var _lifecycleCleanups      = [];

    function _registerLifecycleCleanup(fn) {
      if (_isFn(fn)) _lifecycleCleanups.push(fn);
    }

    function _runLifecycleCleanups() {
      if (!_lifecycleCleanups.length) return;
      var cleanups = _lifecycleCleanups.slice().reverse();
      _lifecycleCleanups.length = 0;
      for (var i = 0; i < cleanups.length; i++) {
        try { cleanups[i](); } catch (_) { /* swallow */ }
      }
    }

    // D37p-graph-interaction-mode-engine-impl — Generic Cytoscape
    // interaction mode controller. The engine owns Cytoscape state;
    // graph lenses supply mode configuration through the shared toolbar
    // and call these narrow handle methods. No lens vocabulary belongs
    // here.
    var _interactionModeId = '';
    var _interactionModeOptions = {};

    function _setCyBoolean(methodName, value) {
      if (!_isFn(cy[methodName])) return;
      try { cy[methodName](!!value); } catch (_) { /* swallow */ }
    }

    function _setNodesGrabbable(enabled) {
      try {
        var nodes = cy.nodes();
        if (!nodes) return;
        if (enabled && _isFn(nodes.grabify)) {
          nodes.grabify();
          return;
        }
        if (!enabled && _isFn(nodes.ungrabify)) {
          nodes.ungrabify();
          return;
        }
        for (var i = 0; i < nodes.length; i++) {
          var n = nodes[i];
          if (enabled && n && _isFn(n.grabify)) n.grabify();
          else if (!enabled && n && _isFn(n.ungrabify)) n.ungrabify();
        }
      } catch (_) { /* swallow */ }
    }

    function _normaliseInteractionOptions(modeId, modeOptions) {
      var optsIn = _isPlainObject(modeOptions) ? modeOptions : {};
      var cyOpts = _isPlainObject(optsIn.cytoscapeOptions) ? optsIn.cytoscapeOptions : optsIn;
      var id = _str(modeId || optsIn.id || '');
      if (id === 'select') {
        return {
          userPanningEnabled: cyOpts.userPanningEnabled !== false,
          boxSelectionEnabled: cyOpts.boxSelectionEnabled === true,
          nodesGrabbable: cyOpts.nodesGrabbable === true,
          autounselectify: cyOpts.autounselectify === true,
        };
      }
      return {
        userPanningEnabled: cyOpts.userPanningEnabled !== false,
        boxSelectionEnabled: cyOpts.boxSelectionEnabled === true,
        nodesGrabbable: cyOpts.nodesGrabbable === true,
        autounselectify: cyOpts.autounselectify === true,
      };
    }

    function _applyInteractionMode(modeId, modeOptions) {
      if (_destroyed) return false;
      var nextId = _str(modeId || '');
      if (!nextId) return false;
      var nextOptions = _normaliseInteractionOptions(nextId, modeOptions);
      _interactionModeId = nextId;
      _interactionModeOptions = nextOptions;
      _setCyBoolean('userPanningEnabled', nextOptions.userPanningEnabled);
      _setCyBoolean('boxSelectionEnabled', nextOptions.boxSelectionEnabled);
      _setCyBoolean('autounselectify', nextOptions.autounselectify);
      _setNodesGrabbable(nextOptions.nodesGrabbable);
      try {
        container.setAttribute('data-interaction-mode', nextId);
        container.setAttribute('data-nodes-grabbable', nextOptions.nodesGrabbable ? 'true' : 'false');
        container.setAttribute('data-box-selection-enabled', nextOptions.boxSelectionEnabled ? 'true' : 'false');
      } catch (_) { /* swallow */ }
      return true;
    }

    function _resetInteractionMode() {
      _interactionModeId = '';
      _interactionModeOptions = {};
      _setCyBoolean('userPanningEnabled', true);
      _setCyBoolean('boxSelectionEnabled', false);
      _setCyBoolean('autounselectify', false);
      _setNodesGrabbable(false);
      try {
        container.removeAttribute('data-interaction-mode');
        container.removeAttribute('data-nodes-grabbable');
        container.removeAttribute('data-box-selection-enabled');
      } catch (_) { /* swallow */ }
    }

    var handle = {
      destroy: function () {
        if (_destroyed) return;
        _destroyed = true;
        // D37s-context-geometry-2-impl — cancel any pending
        // measurement coalesce / overlap validation BEFORE tearing
        // down so handlers can't fire on stale state.
        if (_measurementChangeRaf) {
          try {
            if (typeof window.cancelAnimationFrame === 'function') {
              window.cancelAnimationFrame(_measurementChangeRaf);
            } else {
              clearTimeout(_measurementChangeRaf);
            }
          } catch (_) { /* swallow */ }
          _measurementChangeRaf = 0;
        }
        if (_overlapValidationTimer) {
          try { clearTimeout(_overlapValidationTimer); } catch (_) { /* swallow */ }
          _overlapValidationTimer = 0;
        }
        // D37u-cytoscape-resize-policy-impl — No deferred resize
        // state to clean up. The passive lifecycle is synchronous
        // (cy.resize() per tick); there is no pending timer or
        // cached rect to invalidate.

        // D37af-initial-fit-stability-gate — Cancel the safety-reveal
        // timer and any pending stabilisation rAF / fallback timer
        // so late ticks can't run after teardown.
        if (_initialFitRevealTimer) {
          try { clearTimeout(_initialFitRevealTimer); } catch (_) { /* swallow */ }
          _initialFitRevealTimer = 0;
        }
        if (_stabilisationRaf) {
          try {
            if (typeof window.cancelAnimationFrame === 'function') {
              window.cancelAnimationFrame(_stabilisationRaf);
            } else {
              clearTimeout(_stabilisationRaf);
            }
          } catch (_) { /* swallow */ }
          _stabilisationRaf = 0;
        }
        // Order matters: lens lifecycle cleanups first, while cy is
        // still live, then overlay, camera-bus deregistration, cy
        // destroy, and DOM removal.
        _runLifecycleCleanups();
        _resetInteractionMode();
        if (overlayHandle && _isFn(overlayHandle.destroy)) {
          try { overlayHandle.destroy(); } catch (_) { /* swallow */ }
        }
        overlayHandle = null;
        if (resizeObs) {
          try { resizeObs.disconnect(); } catch (_) { /* swallow */ }
        }
        resizeObs = null;
        var g = window.MIDASExplorerGraph;
        if (g && g.graphCameraBus && _isFn(g.graphCameraBus.unregisterLens)) {
          try { g.graphCameraBus.unregisterLens(lensId); } catch (_) { /* swallow */ }
        }
        try { cy.destroy(); } catch (_) { /* swallow */ }
        if (container.parentNode) {
          try { container.parentNode.removeChild(container); } catch (_) { /* swallow */ }
        }
      },
      refresh: function (newData) {
        if (_destroyed || !_isPlainObject(newData)) return;
        var newElements = _toCyElements(newData);
        var savedPan, savedZoom;
        try { savedPan  = cy.pan();  } catch (_) { savedPan  = null; }
        try { savedZoom = cy.zoom(); } catch (_) { savedZoom = null; }
        try {
          cy.batch(function () {
            cy.elements().remove();
            cy.add(newElements);
          });
        } catch (_) { /* swallow */ }
        if (savedZoom != null) { try { cy.zoom(savedZoom); } catch (_) {} }
        if (savedPan  != null) { try { cy.pan(savedPan);   } catch (_) {} }
        if (overlayHandle && _isFn(overlayHandle.refresh)) {
          try { overlayHandle.refresh(); } catch (_) { /* swallow */ }
        }
      },
      getCardEl: function (key) {
        return (overlayHandle && _isFn(overlayHandle.getCardEl))
          ? overlayHandle.getCardEl(key) : null;
      },
      getNode: function (id) {
        if (_destroyed) return null;
        var n;
        try { n = cy.getElementById(_str(id)); } catch (_) { return null; }
        if (!n || !n.length) return null;
        var pos;
        try { pos = n.position(); } catch (_) { pos = { x: 0, y: 0 }; }
        var data;
        try { data = n.data() || {}; } catch (_) { data = {}; }
        return {
          id:       n.id(),
          position: { x: pos.x, y: pos.y },
          kind:     _str(data.kind),
          data:     data,
        };
      },
      zoomIn: function () {
        if (_destroyed) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = _clampZoom(cy.zoom() * ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      zoomOut: function () {
        if (_destroyed) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = _clampZoom(cy.zoom() / ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      fit: function (fitOpts) {
        if (_destroyed) return;
        // D37s-viewport-fit-1-impl — Strategic fit envelope.
        //
        // Priority:
        //   1. `fitOpts.padding` (caller-supplied per-side or scalar)
        //      → take the legacy `_fitWithSafeArea` path with the
        //      caller's explicit padding.
        //   2. `opts.getUsableGraphRect` (strategic) → fit cy bbox
        //      into the platform-supplied usable rectangle.
        //   3. `opts.getSafeArea` (legacy fallback) → compose insets
        //      with `DEFAULT_FIT_PADDING` floor and apply per-side.
        //   4. Uniform `DEFAULT_FIT_PADDING` on all sides.
        //
        // D37u-cytoscape-resize-policy-impl — Caller may pass
        // `fitOpts.source` (e.g. `'explicit_fit'`, `'reset_view'`,
        // `'zoom_to_selected'`, `'lens_activation'`, `'data_reload'`)
        // to attribute the camera mutation in the `engine_fit_invoked`
        // diagnostic. Default is `'explicit_fit'`.
        var source = (fitOpts && _isPlainObject(fitOpts) && typeof fitOpts.source === 'string' && fitOpts.source)
          ? fitOpts.source : 'explicit_fit';
        var padInput = (fitOpts && _isPlainObject(fitOpts)) ? fitOpts.padding : undefined;
        if (padInput !== undefined) {
          _fitWithSafeArea(cy, _resolveFitPadding(padInput, opts.getSafeArea));
          _emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, { source: source });
          // D37ag-diagnostic-geometry-dump — observational only.
          _emitGeometryDump('explicit_fit', source, null);
          return;
        }
        if (_isFn(opts.getUsableGraphRect)) {
          var usable = null;
          try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
          if (_isPlainObject(usable) && usable.width > 0 && usable.height > 0) {
            if (_fitToUsableRect(cy, usable, _recordFitDiagnostic)) {
              _emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, { source: source });
              // D37ag-diagnostic-geometry-dump — observational only.
              _emitGeometryDump('explicit_fit', source, null);
              return;
            }
          } else if (_isFn(_recordFitDiagnostic)) {
            _recordFitDiagnostic(DIAG_USABLE_RECT_EMPTY);
          }
        }
        _fitWithSafeArea(cy, _resolveFitPadding(undefined, opts.getSafeArea));
        _emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, { source: source });
        // D37ag-diagnostic-geometry-dump — observational only.
        _emitGeometryDump('explicit_fit', source, null);
      },
      reset: function () {
        if (_destroyed) return;
        try { cy.zoom(1); cy.center(); } catch (_) { /* swallow */ }
      },
      focus: function (nodeId) {
        if (_destroyed) return;
        try {
          var n = cy.getElementById(_str(nodeId));
          if (n && n.length) cy.center(n);
        } catch (_) { /* swallow */ }
      },
      getZoom: function () {
        if (_destroyed) return null;
        try {
          var z = cy.zoom();
          if (typeof z === 'number' && isFinite(z) && z > 0) return z;
        } catch (_) { /* swallow */ }
        return null;
      },
      setZoom: function (z) {
        if (_destroyed) return;
        var next = _clampZoom(z);
        if (next == null) return;
        try {
          var w = cy.width(), h = cy.height();
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      forceRender: function () {
        if (_destroyed) return;
        try { cy.forceRender(); } catch (_) { /* swallow */ }
      },
      // D37s-context-geometry-2-impl — Engine diagnostics surface.
      //
      // Returns the current engine-level diagnostics buffer (overlap
      // detections, etc.). The buffer is append-only within a mount
      // lifetime; consumers (tests, dev tools) can filter by `code`.
      // Returns a SHALLOW COPY so external code can't mutate engine
      // internals.
      getDiagnostics: function () {
        if (_destroyed) return [];
        return _engineDiagnostics.slice();
      },
      setInteractionMode: function (modeId, modeOptions) {
        return _applyInteractionMode(modeId, modeOptions);
      },
      getInteractionMode: function () {
        return _interactionModeId;
      },
      getInteractionModeOptions: function () {
        return _shallowAssign({}, _interactionModeOptions);
      },
      setNodesGrabbable: function (enabled) {
        if (_destroyed) return;
        _setNodesGrabbable(enabled === true);
      },
      clearSelectionSet: function () {
        if (_destroyed) return;
        if (_isFn(_selectionSetClearRequester)) {
          try { _selectionSetClearRequester(); } catch (_) { /* swallow */ }
          return;
        }
        try { cy.nodes(':selected').unselect(); } catch (_) { /* swallow */ }
      },
    };

    // ── Measurement-change forwarding (coalesced) ──
    //
    // D37s-context-geometry-2-impl — When the overlay reports a card
    // measurement, the engine updates `_measurementCache` synchronously
    // (so the cache is fresh within the same frame for tests and
    // dev-tool consumers) but defers the lens callback to the next
    // rAF. Multiple measurement events in the same frame coalesce
    // into a single `onMeasurementsChange(snapshot)` call carrying
    // the FULL current cache. The lens consumes the snapshot and
    // decides whether to reflow (per its own threshold + scheduling
    // policy); the engine does NOT recompute layout itself.
    function _scheduleMeasurementChange() {
      if (_destroyed || !_isFn(opts.onMeasurementsChange)) return;
      if (_measurementChangeRaf) return;
      var raf = (typeof window.requestAnimationFrame === 'function')
        ? window.requestAnimationFrame.bind(window)
        : function (fn) { return setTimeout(fn, 16); };
      _measurementChangeRaf = raf(function () {
        _measurementChangeRaf = 0;
        if (_destroyed) return;
        // Pass a shallow-cloned snapshot so the lens can't mutate
        // engine internals through the callback argument.
        var snapshot = {};
        for (var k in _measurementCache) {
          if (Object.prototype.hasOwnProperty.call(_measurementCache, k)) {
            var v = _measurementCache[k];
            snapshot[k] = { width: v.width, height: v.height };
          }
        }
        try { opts.onMeasurementsChange(snapshot); }
        catch (_) { /* swallow — lens-callback errors must not break engine */ }
      });
    }

    // ── Overlap validation (debounced + dedup'd) ──
    //
    // D37s-context-geometry-2-impl — The engine's global non-overlap
    // invariant: every Cytoscape-backed graph the engine mounts must
    // have non-overlapping card bounding boxes once measured runtime
    // dimensions are available.
    //
    // The validation runs at most once per OVERLAP_VALIDATION_DEBOUNCE_MS
    // window. Within a window:
    //   • The first measurement event arms `_overlapValidationTimer`.
    //   • Subsequent measurement events DO NOT re-arm (the timer is
    //     already pending). They are coalesced into the same pending
    //     validation.
    //   • When the timer fires, validation runs once against the
    //     current cy node positions + data dimensions.
    //   • Each unique overlap pair fires its console warn at most
    //     once per validation; `_overlapSeen` retains the seen-set
    //     across validations until a validation returns zero
    //     overlaps, at which point the seen-set resets (so a re-
    //     emergence after resolution re-fires).
    function _scheduleOverlapValidation() {
      if (_destroyed) return;
      if (_overlapValidationTimer) return;  // already pending
      _overlapValidationTimer = setTimeout(function () {
        _overlapValidationTimer = 0;
        if (_destroyed) return;
        _runOverlapValidation();
      }, OVERLAP_VALIDATION_DEBOUNCE_MS);
    }

    // D37s-viewport-fit-1-impl — Fit-diagnostic recorder.
    //
    // Records a fit-pipeline diagnostic in the engine's
    // `_engineDiagnostics` buffer and emits a deduplicated console
    // warn. Codes are defined as module-level constants
    // (`DIAG_USABLE_RECT_EMPTY`, etc.). Each code fires at most once
    // per mount lifetime — this is the conservative dedup contract.
    // Lens consumers / tests read the buffer via
    // `handle.getDiagnostics()` and filter by `code`.
    function _recordFitDiagnostic(code) {
      if (_destroyed || !code) return;
      if (_fitDiagnosticsSeen[code]) return;
      _fitDiagnosticsSeen[code] = true;
      var ts = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _engineDiagnostics.push({ code: code, ts: ts });
      if (typeof console !== 'undefined' && typeof console.warn === 'function') {
        try {
          console.warn('[graph-cytoscape-engine] ' + code, { lensId: lensId });
        } catch (_) { /* swallow */ }
      }
    }

    // D37x-engine-node-geometry-contract — Emit the dedup'd geometry
    // diagnostic now that `_recordFitDiagnostic` is in scope. The
    // pre-mount scan above set `_nodeGeometryViolated` if any
    // supplied node lacked positive finite width/height. Emitting it
    // here means the violation is observable via
    // `handle.getDiagnostics()` immediately after mount returns.
    if (_nodeGeometryViolated) {
      _recordFitDiagnostic(DIAG_NODE_GEOMETRY_INVALID);
    }

    function _runOverlapValidation() {
      if (_destroyed) return;
      var result = _validateNoOverlap(cy);
      var now = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _overlapValidationT = now;
      if (!result.overlaps.length) {
        // Reset seen-set so a future re-emergence re-fires warnings.
        _overlapSeen = {};
        return;
      }
      // Emit deduplicated diagnostics — both console warn AND the
      // engine diagnostics buffer.
      for (var i = 0; i < result.overlaps.length; i++) {
        var ov = result.overlaps[i];
        // Canonical pair key (sorted) so 'A|B' and 'B|A' don't fire
        // twice on different validation runs.
        var pairKey = (String(ov.cardA) < String(ov.cardB))
          ? (ov.cardA + '|' + ov.cardB)
          : (ov.cardB + '|' + ov.cardA);
        if (_overlapSeen[pairKey]) continue;
        _overlapSeen[pairKey] = true;
        _engineDiagnostics.push({
          code:  'card_overlap_detected',
          cardA: ov.cardA,
          cardB: ov.cardB,
          ts:    now,
        });
        if (typeof console !== 'undefined' && typeof console.warn === 'function') {
          try {
            console.warn(
              '[graph-cytoscape-engine] card_overlap_detected — supplied positions + measured dims produce overlapping bounding boxes; lens must supply non-overlapping positions or larger spacing',
              { lensId: lensId, cardA: ov.cardA, cardB: ov.cardB }
            );
          } catch (_) { /* swallow */ }
        }
      }
    }

    // ── Selection tap routing ──
    //
    // The engine subscribes to cy node taps and forwards them to the
    // lens's adapter. This is the SOLE cy event subscription the
    // engine wires for the lens's selection contract. The lens
    // adapter is responsible for translating the event into whatever
    // bridge call the lens needs (e.g. `contextSelectionBridge.
    // selectCard`). The engine does not name any lens bridge.
    try {
      cy.on('tap', 'node', function (evt) {
        try { opts.selectionAdapter(evt, handle); } catch (_) { /* swallow */ }
      });
    } catch (_) { /* swallow — selection tap wiring must not block mount */ }

    // ── Selection-set routing ──
    //
    // Optional, lens-neutral bridge for box/multi-select customers.
    // The engine owns the raw Cytoscape selection subscriptions and
    // forwards a coalesced selected-node collection to the lens. The
    // lens maps nodes into its own descriptor vocabulary and publishes
    // through the shared selection bridge.
    if (_isFn(opts.selectionSetAdapter)) {
      (function () {
        var selectionSetFrame = 0;
        var selectionSetTimer = 0;
        var selectionSetPrimaryId = null;
        var disposed = false;

        function cancelSelectionSetPublish() {
          if (selectionSetFrame && typeof window.cancelAnimationFrame === 'function') {
            try { window.cancelAnimationFrame(selectionSetFrame); } catch (_) { /* swallow */ }
          }
          if (selectionSetTimer && typeof window.clearTimeout === 'function') {
            try { window.clearTimeout(selectionSetTimer); } catch (_) { /* swallow */ }
          }
          selectionSetFrame = 0;
          selectionSetTimer = 0;
        }

        function selectedNodes() {
          try { return cy.nodes(':selected'); }
          catch (_) { return null; }
        }

        function publishSelectionSet() {
          selectionSetFrame = 0;
          selectionSetTimer = 0;
          if (disposed || _destroyed) return;
          var event = {
            type:          'selection',
            selectedNodes: selectedNodes(),
            primaryId:     selectionSetPrimaryId,
            cytoscape:     cy,
          };
          selectionSetPrimaryId = null;
          try { opts.selectionSetAdapter(event, handle); } catch (_) { /* swallow */ }
        }

        function scheduleSelectionSet(primaryId) {
          if (primaryId) selectionSetPrimaryId = primaryId;
          if (selectionSetFrame || selectionSetTimer) return;
          if (typeof window.requestAnimationFrame === 'function') {
            selectionSetFrame = window.requestAnimationFrame(publishSelectionSet);
          } else {
            selectionSetTimer = window.setTimeout(publishSelectionSet, 0);
          }
        }

        function clearSelectionSet() {
          cancelSelectionSetPublish();
          selectionSetPrimaryId = null;
          try {
            opts.selectionSetAdapter({
              type:          'clear',
              selectedNodes: selectedNodes(),
              primaryId:     null,
              cytoscape:     cy,
            }, handle);
          } catch (_) { /* swallow */ }
        }

        function clearVisualSelectionSet() {
          try { cy.nodes(':selected').unselect(); } catch (_) { /* swallow */ }
          clearSelectionSet();
        }

        _selectionSetClearRequester = clearVisualSelectionSet;

        function onNodeTap(evt) {
          var id = '';
          try { id = evt && evt.target ? String(evt.target.id() || '') : ''; } catch (_) { id = ''; }
          scheduleSelectionSet(id || null);
        }

        function onSelectionChange() {
          scheduleSelectionSet(null);
        }

        function onCoreTap(evt) {
          if (!evt || evt.target !== cy) return;
          if (_interactionMode && _interactionMode !== 'select') return;
          clearVisualSelectionSet();
        }

        function onKeydown(evt) {
          if (!evt) return;
          var key = evt.key || '';
          if (key !== 'Escape' && key !== 'Esc') return;
          clearVisualSelectionSet();
        }

        try { cy.on('tap', 'node', onNodeTap); } catch (_) { /* swallow */ }
        try { cy.on('select unselect', 'node', onSelectionChange); } catch (_) { /* swallow */ }
        try { cy.on('tap', onCoreTap); } catch (_) { /* swallow */ }
        try { window.addEventListener('keydown', onKeydown); } catch (_) { /* swallow */ }

        _registerLifecycleCleanup(function () {
          disposed = true;
          if (_selectionSetClearRequester === clearVisualSelectionSet) {
            _selectionSetClearRequester = null;
          }
          cancelSelectionSetPublish();
          try { cy.off('tap', 'node', onNodeTap); } catch (_) { /* swallow */ }
          try { cy.off('select unselect', 'node', onSelectionChange); } catch (_) { /* swallow */ }
          try { cy.off('tap', onCoreTap); } catch (_) { /* swallow */ }
          try { window.removeEventListener('keydown', onKeydown); } catch (_) { /* swallow */ }
          clearSelectionSet();
        });
      })();
    }

    // ── Camera-bus registration ──
    //
    // The lens supplies a `cameraAdapter(handle)` factory. The engine
    // calls it immediately, captures the returned delegate, and
    // registers it with `graphCameraBus` under `lensId`. This is
    // the canonical wiring path; lenses no longer call
    // `graphCameraBus.registerLens(...)` directly from their own
    // code. Bus deregistration happens in `handle.destroy()` above.
    try {
      var cameraDelegate = opts.cameraAdapter(handle);
      if (_isPlainObject(cameraDelegate)) {
        var g = window.MIDASExplorerGraph;
        if (g && g.graphCameraBus && _isFn(g.graphCameraBus.registerLens)) {
          try { g.graphCameraBus.registerLens(lensId, cameraDelegate); }
          catch (_) { /* swallow */ }
        }
      }
    } catch (_) { /* swallow — camera wiring must not block mount */ }

    // ── Overlay mount via the shared overlay module ──
    //
    // The engine uses the existing `graph-cytoscape-overlay.js`
    // shared overlay module (tranche B') as its overlay component.
    // The overlay's mount is internal to the engine — no lens calls
    // it directly. The overlay layer is appended to the engine's
    // container as a sibling of `cyMount`, giving both the same
    // positioning context and `inset: 0` coordinate frame. This
    // alignment is the structural fix for the geometry-mismatch bug
    // that motivated this tranche.
    //
    // D37s-context-geometry-diagnostic — opt-out switch. When the
    // lens passes `overlayEnabled: false` to `engine.mount(...)`, the
    // engine skips the overlay-mount call entirely. The cy canvas
    // continues to render normally; lenses that opt out are
    // responsible for supplying a `nodeStyleOverride` that makes raw
    // cy nodes visible (the engine's base style is transparent
    // because it assumes the overlay will paint the visible cards).
    // Default is `true` (back-compat) — Authority is unaffected.
    var g = window.MIDASExplorerGraph;
    var overlayEnabled = (opts.overlayEnabled !== false);
    if (overlayEnabled && g && g.graphCytoscapeOverlay && _isFn(g.graphCytoscapeOverlay.mount)) {
      try {
        overlayHandle = g.graphCytoscapeOverlay.mount(cy, container, {
          lensId:         lensId,
          template:       opts.template,
          keyForNode:     opts.keyForNode,
          stateClasses:   _isPlainObject(opts.stateClasses) ? opts.stateClasses : { selected: 'is-selected', hover: 'is-hover' },
          syncSelected:   opts.syncSelected !== false,
          syncHover:      opts.syncHover !== false,
          pointerEvents:  _str(opts.pointerEvents) || 'none',
          layerClassName: _str(opts.layerClassName),
          // D37s-context-geometry-1-impl — Engine consumes overlay
          // measurements and propagates them to cy node dimensions
          // via `_propagateDimensions`. So that cy's edge routing
          // clips to the visible card boundary.
          //
          // D37s-context-geometry-2-impl — Engine ALSO updates a
          // measurement cache + forwards a coalesced view to the
          // lens via `opts.onMeasurementsChange` (if supplied), and
          // schedules a debounced non-overlap validation pass.
          // Order matters:
          //   1. Update cache + propagate to cy (data write).
          //   2. Coalesce lens notification to next rAF.
          //   3. Schedule (or refresh) overlap validation debounce.
          onMeasure: function (key, w, h) {
            if (!key || !(w > 0) || !(h > 0)) return;
            _propagateDimensions(cy, key, w, h);
            _measurementCache[String(key)] = { width: w, height: h };
            _scheduleMeasurementChange();
            _scheduleOverlapValidation();
          },
        });
      } catch (_) { overlayHandle = null; }
    }

    if (_isFn(opts.onReady)) {
      try {
        var readyCleanup = opts.onReady({
          cytoscape: cy,
          handle: handle,
          container: container,
          registerCleanup: _registerLifecycleCleanup,
        });
        if (_isFn(readyCleanup)) _registerLifecycleCleanup(readyCleanup);
      } catch (_) { /* swallow — lens lifecycle hooks must not block mount */ }
    }

    // ── Per-firing lifecycle diagnostic emitter ──
    //
    // The fit-pipeline diagnostics (`usable_rect_empty`,
    // `fit_zoom_clamped_*`, etc.) emit at most ONCE per mount via
    // `_recordFitDiagnostic` so a chatty consumer doesn't spam the
    // console. The resize / fit LIFECYCLE codes have the opposite
    // need: external observers (tests, dev tools, browser console
    // during a drawer toggle) must be able to count events.
    // `_emitLifecycleDiagnostic` writes to the same diagnostics
    // buffer and console channel as `_recordFitDiagnostic` but skips
    // the dedup map.
    //
    // D37u-cytoscape-resize-policy-impl — Optional `payload` carries
    // structured context (e.g. `{ source: 'initial_mount' }` for
    // `engine_fit_invoked`). The payload is merged into the diagnostic
    // entry and the console warn so downstream consumers can tell
    // why a camera mutation occurred.
    function _emitLifecycleDiagnostic(code, payload) {
      if (_destroyed || !code) return;
      var ts = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      var entry = { code: code, ts: ts };
      if (_isPlainObject(payload)) {
        for (var k in payload) {
          if (Object.prototype.hasOwnProperty.call(payload, k)) entry[k] = payload[k];
        }
      }
      _engineDiagnostics.push(entry);
      if (typeof console !== 'undefined' && typeof console.warn === 'function') {
        try {
          var logCtx = { lensId: lensId };
          if (_isPlainObject(payload)) {
            for (var k2 in payload) {
              if (Object.prototype.hasOwnProperty.call(payload, k2)) logCtx[k2] = payload[k2];
            }
          }
          console.warn('[graph-cytoscape-engine] ' + code, logCtx);
        } catch (_) { /* swallow */ }
      }
    }

    // ── Shared fit pipeline ──
    //
    // D37u-cytoscape-resize-policy-impl — Used by EXPLICIT fit
    // triggers only (initial settle, `handle.fit()`, future operator
    // actions). NEVER called by the passive resize lifecycle.
    //
    // The pipeline:
    //
    //   1. cy.resize()  — re-reads container dimensions (cheap; no
    //                     camera change).
    //   2. If `opts.getUsableGraphRect` returns a non-empty rect:
    //      fit via `_fitToUsableRect` (strategic path).
    //   3. Otherwise fall back to `_fitWithSafeArea` with per-side
    //      padding resolved from `opts.getSafeArea`.
    //
    // The `source` argument names the trigger (`initial_mount`,
    // `explicit_fit`, `reset_view`, `zoom_to_selected`, etc.) and is
    // emitted on the `engine_fit_invoked` diagnostic so dev tools can
    // attribute camera changes.
    function _runFitPipeline(source, phase) {
      if (_destroyed) return false;
      var fitApplied = false;
      try {
        cy.resize();
        var fittedViaUsable = false;
        if (_isFn(opts.getUsableGraphRect)) {
          var usable = null;
          try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
          if (_isPlainObject(usable) && usable.width > 0 && usable.height > 0) {
            fittedViaUsable = _fitToUsableRect(cy, usable, _recordFitDiagnostic);
          } else if (_isFn(_recordFitDiagnostic)) {
            _recordFitDiagnostic(DIAG_USABLE_RECT_EMPTY);
          }
        }
        if (!fittedViaUsable) {
          var padding = _resolveFitPadding(undefined, opts.getSafeArea);
          _fitWithSafeArea(cy, padding);
        }
        fitApplied = true;
        // D37ad-initial-fit-stable-cadence — Optional `phase` payload
        // distinguishes the initial settle ticks ('raf1' / 'raf2' /
        // 'tail') so multi-fit diagnostics are observable per tick.
        // Explicit fits (handle.fit) omit `phase`.
        var diagPayload = {
          source: (typeof source === 'string' && source) ? source : 'explicit_fit',
        };
        if (typeof phase === 'string' && phase) diagPayload.phase = phase;
        _emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, diagPayload);
      } catch (_) { /* swallow */ }
      return fitApplied;
    }

    // ── Initial fit / reveal lifecycle (stability-driven) ──
    //
    // D37af-initial-fit-stability-gate — Replaces the D37ad fixed
    // cadence (rAF + rAF + 120 ms tail + 500 ms safety) with an
    // animation-frame-driven stabilisation loop. While the graph is
    // hidden, every frame:
    //
    //   1. cy.resize() — re-read live container dimensions.
    //   2. `_takeFitSnapshot()` reads the fit-relevant geometry.
    //   3. If the snapshot is valid, run the canonical fit pipeline.
    //   4. Compare against the previous snapshot.
    //   5. If stable across INITIAL_FIT_STABLE_FRAMES consecutive
    //      frames → reveal.
    //   6. If INITIAL_FIT_SAFETY_MS elapsed → force-reveal.
    //
    // After reveal, `_tryPostRevealCorrection()` (invoked from the
    // passive resize handler) may fire ONE corrective fit if the
    // FIRST resize tick within POST_REVEAL_STABILISATION_MS carries
    // a materially different snapshot, the user hasn't interacted
    // with the camera, and a correction budget is still available.
    // After that, D37u steady-state takes over.

    // ── D37ag-diagnostic-geometry-dump — observational only ──
    //
    // Emits `engine_geometry_dump` with the full set of fit-relevant
    // geometry inputs AND the post-fit camera state at the call site.
    // Used by dev tools to diff initial-mount fit against manual Fit:
    // identical math + differing inputs ⇒ a field-level diff pinpoints
    // which input drifted.
    //
    // The helper MUST NOT mutate engine state. Each measurement is
    // wrapped so a single failing read returns `null` for that field
    // but never throws. Every payload field is present (`null` when
    // unavailable); fields are never omitted, so dev-tool consumers
    // can shape-match the payload.
    function _emitGeometryDump(trigger, source, phase) {
      if (_destroyed) return;
      var cw = null, ch = null;
      try { cw = _isFn(cy.width)  ? cy.width()  : null; } catch (_) { cw = null; }
      try { ch = _isFn(cy.height) ? cy.height() : null; } catch (_) { ch = null; }
      var containerRect = null;
      try {
        if (container && typeof container.getBoundingClientRect === 'function') {
          var r = container.getBoundingClientRect();
          containerRect = {
            left:   (r && typeof r.left   === 'number') ? r.left   : null,
            top:    (r && typeof r.top    === 'number') ? r.top    : null,
            width:  (r && typeof r.width  === 'number') ? r.width  : null,
            height: (r && typeof r.height === 'number') ? r.height : null,
            right:  (r && typeof r.right  === 'number') ? r.right  : null,
            bottom: (r && typeof r.bottom === 'number') ? r.bottom : null,
          };
        }
      } catch (_) { containerRect = null; }
      var usableRect = null;
      try {
        if (_isFn(opts.getUsableGraphRect)) {
          var u = opts.getUsableGraphRect();
          if (_isPlainObject(u)) {
            usableRect = {
              x:      (typeof u.x      === 'number') ? u.x      : null,
              y:      (typeof u.y      === 'number') ? u.y      : null,
              width:  (typeof u.width  === 'number') ? u.width  : null,
              height: (typeof u.height === 'number') ? u.height : null,
            };
          }
        }
      } catch (_) { usableRect = null; }
      var elementsBbox = null;
      try {
        var eles = cy.elements();
        if (eles && eles.length > 0) {
          var bb = eles.boundingBox(FIT_BBOX_OPTS);
          if (bb) {
            elementsBbox = {
              x1: (typeof bb.x1 === 'number') ? bb.x1 : null,
              y1: (typeof bb.y1 === 'number') ? bb.y1 : null,
              x2: (typeof bb.x2 === 'number') ? bb.x2 : null,
              y2: (typeof bb.y2 === 'number') ? bb.y2 : null,
              w:  (typeof bb.w  === 'number') ? bb.w  : null,
              h:  (typeof bb.h  === 'number') ? bb.h  : null,
            };
          }
        }
      } catch (_) { elementsBbox = null; }
      var resolvedPadding = null;
      try { resolvedPadding = _resolveFitPadding(undefined, opts.getSafeArea); }
      catch (_) { resolvedPadding = null; }
      var postFitPan = null;
      try {
        if (_isFn(cy.pan)) {
          var p = cy.pan();
          if (p && typeof p.x === 'number' && typeof p.y === 'number') {
            postFitPan = { x: p.x, y: p.y };
          }
        }
      } catch (_) { postFitPan = null; }
      var postFitZoom = null;
      try {
        if (_isFn(cy.zoom)) {
          var z = cy.zoom();
          if (typeof z === 'number' && isFinite(z)) postFitZoom = z;
        }
      } catch (_) { postFitZoom = null; }
      var ts = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _emitLifecycleDiagnostic(DIAG_ENGINE_GEOMETRY_DUMP, {
        trigger:         (typeof trigger === 'string') ? trigger : null,
        source:          (typeof source  === 'string') ? source  : null,
        phase:           (typeof phase   === 'string') ? phase   : null,
        cyWidth:         (typeof cw === 'number') ? cw : null,
        cyHeight:        (typeof ch === 'number') ? ch : null,
        containerRect:   containerRect,
        usableRect:      usableRect,
        elementsBbox:    elementsBbox,
        resolvedPadding: resolvedPadding,
        postFitPan:      postFitPan,
        postFitZoom:     postFitZoom,
        timestamp:       ts,
      });
    }

    function _takeFitSnapshot() {
      if (_destroyed) return { ok: false, reason: 'destroyed' };
      var cw = 0, ch = 0;
      try { cw = _isFn(cy.width)  ? cy.width()  : 0; } catch (_) { cw = 0; }
      try { ch = _isFn(cy.height) ? cy.height() : 0; } catch (_) { ch = 0; }
      if (!(cw > 0) || !(ch > 0)) return { ok: false, reason: 'zero_container' };
      var eles;
      try { eles = cy.elements(); }
      catch (_) { return { ok: false, reason: 'empty_graph' }; }
      if (!eles || eles.length === 0) return { ok: false, reason: 'empty_graph' };
      var bb;
      try { bb = eles.boundingBox(FIT_BBOX_OPTS); }
      catch (_) { return { ok: false, reason: 'invalid_bbox' }; }
      if (!bb || !(bb.w > 0) || !(bb.h > 0)) return { ok: false, reason: 'invalid_bbox' };
      var usable = null;
      if (_isFn(opts.getUsableGraphRect)) {
        try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
      }
      return {
        ok: true,
        snapshot: {
          cyWidth:      cw,
          cyHeight:     ch,
          usableX:      _isPlainObject(usable) ? (usable.x      || 0) : 0,
          usableY:      _isPlainObject(usable) ? (usable.y      || 0) : 0,
          usableWidth:  _isPlainObject(usable) ? (usable.width  || 0) : 0,
          usableHeight: _isPlainObject(usable) ? (usable.height || 0) : 0,
          bboxX1:       bb.x1,
          bboxY1:       bb.y1,
          bboxX2:       bb.x2,
          bboxY2:       bb.y2,
          bboxWidth:    bb.w,
          bboxHeight:   bb.h,
        },
      };
    }

    function _snapshotsMatch(a, b, epsilon) {
      if (!_isPlainObject(a) || !_isPlainObject(b)) return false;
      var eps = (typeof epsilon === 'number' && epsilon >= 0)
        ? epsilon : INITIAL_FIT_STABILITY_EPSILON_PX;
      var keys = [
        'cyWidth', 'cyHeight',
        'usableX', 'usableY', 'usableWidth', 'usableHeight',
        'bboxX1', 'bboxY1', 'bboxX2', 'bboxY2',
        'bboxWidth', 'bboxHeight',
      ];
      for (var i = 0; i < keys.length; i++) {
        var k = keys[i];
        if (Math.abs((a[k] || 0) - (b[k] || 0)) > eps) return false;
      }
      return true;
    }

    // D37ah-readiness-gated-initial-fit — Generic state-transition
    // helper. Centralises the state_changed lifecycle diagnostic and
    // its co-located geometry dump so every transition emits both.
    //
    //   • Self-transitions (newState === current) are dropped.
    //   • Diagnostic payload carries { from: prevState, to: newState }.
    //   • Geometry dump trigger='state_transition', source=newState,
    //     phase=null (per D37ah spec).
    //
    // Callers MUST go through this helper for any change to
    // `_initialFitState`. Direct assignment is restricted to the
    // initial-value declaration in the per-mount state block.
    function _transitionTo(newState) {
      if (_destroyed) return;
      if (_initialFitState === newState) return;
      var prevState = _initialFitState;
      _initialFitState = newState;
      _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED, {
        from: prevState,
        to:   newState,
      });
      // D37ag-diagnostic-geometry-dump — observational only.
      _emitGeometryDump('state_transition', newState, null);
    }

    // D37ah-readiness-gated-initial-fit — readiness-gated rAF loop.
    //
    // Each frame while state === 'stabilising' the loop checks 9
    // fit-input gates BEFORE attempting any fit:
    //
    //   1. containerRect.width  > 0
    //   2. containerRect.height > 0
    //   3. cy.width()           > 0
    //   4. cy.height()          > 0
    //   5. usableRect.width     > 0   (only when getUsableGraphRect set)
    //   6. usableRect.height    > 0   (only when getUsableGraphRect set)
    //   7. elementsBbox.w       > 0
    //   8. elementsBbox.h       > 0
    //   9. fitApplied = true (from _runFitPipeline)
    //
    // Failing input gates (1-8) emit engine_readiness_gate_failed and
    // reschedule WITHOUT advancing the stability counter. A failed
    // fit pipeline (gate 9) emits engine_readiness_fit_failed and
    // reschedules; the stability counter is NOT advanced because the
    // snapshot wasn't validated against a successful fit.
    //
    // The previous D37af time-based safety cap inside the rAF body
    // has been REMOVED — the single external setTimeout below now
    // owns the safety-cap rule.
    function _runStabilisationFrame() {
      _stabilisationRaf = 0;
      if (_destroyed) return;
      if (_initialFitState !== 'stabilising') return;
      // cy.resize() reads the live container dims; cheap.
      try { cy.resize(); } catch (_) { /* swallow */ }

      // Gates 1-2: container bounding rect.
      var containerRect = null;
      try {
        if (container && typeof container.getBoundingClientRect === 'function') {
          containerRect = container.getBoundingClientRect();
        }
      } catch (_) { containerRect = null; }
      if (!containerRect || !(containerRect.width > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'containerRect.width',
        });
        _scheduleNextStabilisationFrame();
        return;
      }
      if (!(containerRect.height > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'containerRect.height',
        });
        _scheduleNextStabilisationFrame();
        return;
      }

      // Gates 3-4: cy viewport dimensions.
      var cw = 0, ch = 0;
      try { cw = _isFn(cy.width)  ? cy.width()  : 0; } catch (_) { cw = 0; }
      try { ch = _isFn(cy.height) ? cy.height() : 0; } catch (_) { ch = 0; }
      if (!(cw > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'cy.width',
        });
        _scheduleNextStabilisationFrame();
        return;
      }
      if (!(ch > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'cy.height',
        });
        _scheduleNextStabilisationFrame();
        return;
      }

      // Gates 5-6: usable graph rect (only when provided).
      if (_isFn(opts.getUsableGraphRect)) {
        var usable = null;
        try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
        if (!_isPlainObject(usable) || !(usable.width > 0)) {
          _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
            gate: 'usableRect.width',
          });
          _scheduleNextStabilisationFrame();
          return;
        }
        if (!(usable.height > 0)) {
          _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
            gate: 'usableRect.height',
          });
          _scheduleNextStabilisationFrame();
          return;
        }
      }

      // Gates 7-8: elements bounding box.
      var eles;
      try { eles = cy.elements(); } catch (_) { eles = null; }
      if (!eles || eles.length === 0) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'elementsBbox.empty',
        });
        _scheduleNextStabilisationFrame();
        return;
      }
      var bb;
      try { bb = eles.boundingBox(FIT_BBOX_OPTS); } catch (_) { bb = null; }
      if (!bb || !(bb.w > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'elementsBbox.w',
        });
        _scheduleNextStabilisationFrame();
        return;
      }
      if (!(bb.h > 0)) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {
          gate: 'elementsBbox.h',
        });
        _scheduleNextStabilisationFrame();
        return;
      }

      // All input gates passed; take the snapshot used for stability
      // comparison and run the canonical fit pipeline.
      var sample = _takeFitSnapshot();
      if (!sample.ok) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_SAMPLE, {
          ok: false, reason: sample.reason,
        });
        _scheduleNextStabilisationFrame();
        return;
      }

      // Gate 9: canonical fit pipeline. Phase remains 'stabilising'
      // so the D37u source/phase pin survives.
      var fitApplied = _runFitPipeline('initial_mount', 'stabilising');
      // D37ag-diagnostic-geometry-dump — observational only.
      _emitGeometryDump('stabilisation_tick', 'initial_mount', 'stabilising');
      if (!fitApplied) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_FIT_FAILED);
        _scheduleNextStabilisationFrame();
        return;
      }

      if (_lastFitSnapshot == null) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_FIT_APPLIED);
      }
      _fitPipelineAppliedAtLeastOnce = true;

      // Stability count: increment if this snapshot matches the
      // previous; reset to 1 (current frame is the new baseline)
      // otherwise.
      if (_lastFitSnapshot && _snapshotsMatch(sample.snapshot, _lastFitSnapshot)) {
        _stableFrameCount++;
      } else {
        _stableFrameCount = 1;
      }
      _lastFitSnapshot = sample.snapshot;
      _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_SAMPLE, {
        ok: true, stableFrames: _stableFrameCount,
      });
      if (_stableFrameCount >= INITIAL_FIT_STABLE_FRAMES) {
        _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STABLE, {
          frames: _stableFrameCount,
        });
        _revealSnapshot = sample.snapshot;
        _revealGraph('clean');
        return;
      }
      _scheduleNextStabilisationFrame();
    }

    function _scheduleNextStabilisationFrame() {
      if (_destroyed) return;
      if (_initialFitState !== 'stabilising') return;
      if (_stabilisationRaf) return;
      if (typeof window.requestAnimationFrame === 'function') {
        _stabilisationRaf = window.requestAnimationFrame(_runStabilisationFrame);
      } else {
        _stabilisationRaf = setTimeout(_runStabilisationFrame, 16);
      }
    }

    function _bindUserInteractionGuards() {
      if (_userInteractionGuardBound) return;
      _userInteractionGuardBound = true;
      // Any operator-driven camera change (or programmatic camera
      // change like an explicit handle.fit) fires these cy events.
      // After any such event the post-reveal correction is disabled
      // so the operator's intent is never overwritten.
      try {
        cy.on('pan zoom drag tap', function () { _userInteracted = true; });
      } catch (_) { /* swallow */ }
    }

    // D37ah-readiness-gated-initial-fit — Reason-based reveal.
    //
    // `reason` MUST be one of 'clean' | 'forced_cy_zero' |
    // 'forced_fit_failed'. Any other value is treated as 'clean'
    // (defensive default — the safety-cap fan-out validates the
    // value at the call site).
    //
    // Mapping:
    //   'clean'             → state 'revealed';
    //                         data-initial-fit='complete';
    //                         DIAG_ENGINE_INITIAL_REVEAL.
    //   'forced_cy_zero'    → state 'forced_reveal_cy_zero';
    //                         data-initial-fit='failed-reveal-cy-zero';
    //                         DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO.
    //   'forced_fit_failed' → state 'forced_reveal_fit_failed';
    //                         data-initial-fit='failed-reveal-fit';
    //                         DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED.
    //
    // NOTE: 'blocked_zero_container' is NOT a reveal reason — it is
    // a terminal state reached via _transitionTo directly, with the
    // container remaining hidden.
    function _revealGraph(reason) {
      if (_destroyed) return;
      if (_initialFitState === 'revealed' ||
          _initialFitState === 'forced_reveal_cy_zero' ||
          _initialFitState === 'forced_reveal_fit_failed' ||
          _initialFitState === 'blocked_zero_container') return;
      var safeReason = (reason === 'clean' ||
                        reason === 'forced_cy_zero' ||
                        reason === 'forced_fit_failed')
        ? reason : 'clean';
      _revealReason = safeReason;
      var newState;
      var attrValue;
      var revealDiag;
      if (safeReason === 'clean') {
        newState   = 'revealed';
        attrValue  = 'complete';
        revealDiag = DIAG_ENGINE_INITIAL_REVEAL;
      } else if (safeReason === 'forced_cy_zero') {
        newState   = 'forced_reveal_cy_zero';
        attrValue  = 'failed-reveal-cy-zero';
        revealDiag = DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO;
      } else {
        newState   = 'forced_reveal_fit_failed';
        attrValue  = 'failed-reveal-fit';
        revealDiag = DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED;
      }
      _transitionTo(newState);
      try {
        container.style.visibility = '';
        container.setAttribute('data-initial-fit', attrValue);
      } catch (_) { /* swallow */ }
      if (_initialFitRevealTimer) {
        try { clearTimeout(_initialFitRevealTimer); } catch (_) { /* swallow */ }
        _initialFitRevealTimer = 0;
      }
      if (_stabilisationRaf) {
        try {
          if (typeof window.cancelAnimationFrame === 'function') {
            window.cancelAnimationFrame(_stabilisationRaf);
          } else {
            clearTimeout(_stabilisationRaf);
          }
        } catch (_) { /* swallow */ }
        _stabilisationRaf = 0;
      }
      // Open the post-reveal grace window.
      var now = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _postRevealGraceEndTs = now + POST_REVEAL_STABILISATION_MS;
      _bindUserInteractionGuards();
      _emitLifecycleDiagnostic(revealDiag);
      // D37ag-diagnostic-geometry-dump — observational only.
      _emitGeometryDump('initial_reveal', null, null);
    }

    // Post-reveal correction. Returns true iff a corrective fit was
    // applied — caller (the passive RO handler) then SKIPS its
    // camera-preserved diagnostic for this tick because the camera
    // was, in fact, mutated.
    //
    // D37ah-readiness-gated-initial-fit — Eligibility (ALL must hold):
    //   • state is 'revealed' OR 'forced_reveal_cy_zero'. The cy-zero
    //     forced reveal is correction-eligible because a late RO tick
    //     may finally deliver non-zero cy dimensions and a corrective
    //     fit becomes valuable. 'forced_reveal_fit_failed' and
    //     'blocked_zero_container' are NOT eligible (fit attempted and
    //     failed, or no reveal at all).
    //   • _postRevealCorrectionsRemaining > 0.
    //   • _userInteracted === false (no pan / zoom / drag / tap
    //     observed after reveal).
    //   • Date.now() <= _postRevealGraceEndTs.
    //   • A fresh valid snapshot is materially different from the
    //     snapshot recorded at reveal time (or any previous snapshot
    //     was recorded if reveal was forced_cy_zero).
    function _tryPostRevealCorrection() {
      if (_initialFitState !== 'revealed' &&
          _initialFitState !== 'forced_reveal_cy_zero') return false;
      if (_postRevealCorrectionsRemaining <= 0) return false;
      if (_userInteracted) return false;
      var now = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      if (now > _postRevealGraceEndTs) return false;
      var sample = _takeFitSnapshot();
      if (!sample.ok) return false;
      if (_revealSnapshot && _snapshotsMatch(sample.snapshot, _revealSnapshot)) return false;
      _postRevealCorrectionsRemaining--;
      _runFitPipeline('post_reveal_stabilisation', null);
      _revealSnapshot = sample.snapshot;
      _emitLifecycleDiagnostic(DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT);
      // D37ag-diagnostic-geometry-dump — observational only.
      _emitGeometryDump('post_reveal_correction', 'post_reveal_stabilisation', null);
      return true;
    }

    // D37ai-blocked-container-recovery — Single external safety-cap
    // helper. (Extracted from the inline mount-time setTimeout so
    // blocked → stabilising recovery can RE-ARM the cap from the
    // recovery moment, not from mount time.) Idempotent: any
    // pending timer is cleared before a new one is armed.
    //
    // Three-way SAFETY-CAP RULE at firing time (unchanged from D37ah):
    //   1. 'awaiting_container' → _transitionTo('blocked_zero_container');
    //      data-initial-fit='blocked-zero-container'; emit
    //      DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED. NO reveal —
    //      the container remained zero-sized and the graph stays
    //      hidden. The ResizeObserver-driven recovery path
    //      (_onContainerResize, 'blocked_zero_container' branch) is
    //      responsible for re-arming this cap when a later resize
    //      tick delivers a measurable container.
    //   2. 'stabilising' AND cy.width/height === 0 →
    //      _revealGraph('forced_cy_zero').
    //   3. 'stabilising' AND cy.width/height > 0 →
    //      _revealGraph('forced_fit_failed').
    //
    // If state has advanced past 'awaiting_container' / 'stabilising'
    // (clean reveal already happened, or already blocked again by a
    // previous cap), the cap is a no-op.
    function _armInitialSafetyCap() {
      if (_destroyed) return;
      if (_initialFitRevealTimer) {
        try { clearTimeout(_initialFitRevealTimer); } catch (_) { /* swallow */ }
        _initialFitRevealTimer = 0;
      }
      _initialFitRevealTimer = setTimeout(function () {
        _initialFitRevealTimer = 0;
        if (_destroyed) return;
        if (_initialFitState === 'awaiting_container') {
          _transitionTo('blocked_zero_container');
          try {
            container.setAttribute('data-initial-fit', 'blocked-zero-container');
          } catch (_) { /* swallow */ }
          _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED);
          return;
        }
        if (_initialFitState !== 'stabilising') return;
        var capCw = 0, capCh = 0;
        try { capCw = _isFn(cy.width)  ? cy.width()  : 0; } catch (_) { capCw = 0; }
        try { capCh = _isFn(cy.height) ? cy.height() : 0; } catch (_) { capCh = 0; }
        if (!(capCw > 0) || !(capCh > 0)) {
          _revealGraph('forced_cy_zero');
          return;
        }
        _revealGraph('forced_fit_failed');
      }, INITIAL_FIT_SAFETY_MS);
    }

    // ── ResizeObserver / resize-only passive lifecycle ──
    //
    // D37u-cytoscape-resize-policy-impl — Passive container resize
    // (drawer toggle, tray expand/collapse, focus-mode chrome change,
    // window resize) MUST preserve the operator's camera. The
    // upstream Cytoscape model is:
    //
    //   • cy.resize() invalidates the cached container size and
    //     schedules a redraw; it does NOT touch _private.pan or
    //     _private.zoom.
    //   • cy.fit() and cy.viewport({zoom, pan}) DO mutate camera and
    //     are therefore CAMERA TRIGGERS — they belong at explicit
    //     events (initial mount, operator Fit / Reset button, lens
    //     activation that explicitly requests a recompose, etc.).
    //
    // STEADY-STATE passive lifecycle reduces to:
    //
    //   1. Emit engine_resize_detected.
    //   2. Call cy.resize().
    //   3. Emit engine_cy_resize_called.
    //   4. Emit engine_camera_preserved_on_resize.
    //
    // The operator's pan/zoom survives across CSS transitions because
    // the engine never touches them.
    //
    // D37ai-blocked-container-recovery — INITIAL-STABILISATION
    // lifecycle is NOT steady state. While in 'awaiting_container',
    // 'blocked_zero_container', or 'stabilising', the resize handler
    // routes recovery (where applicable) but does NOT emit the
    // engine_camera_preserved_on_resize diagnostic. That diagnostic
    // represents steady-state intent and would otherwise mask the
    // fact that the engine is still trying to reach a valid initial
    // fit.
    function _onContainerResize() {
      if (_destroyed) return;
      _emitLifecycleDiagnostic(DIAG_ENGINE_RESIZE_DETECTED);
      // D37ai-blocked-container-recovery — State-branching dispatch.
      //
      //   'awaiting_container'      → check container measurability;
      //                                promote to 'stabilising' when
      //                                measurable. No cy.resize() /
      //                                cy_resize_called /
      //                                camera_preserved emission —
      //                                initial-stabilisation work,
      //                                not steady state.
      //   'blocked_zero_container'  → RECOVERY PATH (new in D37ai).
      //                                Check container measurability;
      //                                if measurable, restart initial
      //                                stabilisation with fresh
      //                                state and a re-armed safety
      //                                cap. The graph stays hidden
      //                                until the readiness loop
      //                                reveals cleanly. No
      //                                camera_preserved emission.
      //   'stabilising'             → the readiness rAF loop already
      //                                owns its own pacing; resize
      //                                tick is a no-op. No
      //                                camera_preserved emission
      //                                (initial-stabilisation phase).
      //   'revealed' /
      //   'forced_reveal_cy_zero' /
      //   'forced_reveal_fit_failed'
      //                             → D37u steady-state path.
      //                                cy.resize(); emit
      //                                cy_resize_called; try
      //                                post-reveal correction; if it
      //                                doesn't fire, emit
      //                                camera_preserved.
      if (_initialFitState === 'awaiting_container') {
        var cr = null;
        try {
          if (container && typeof container.getBoundingClientRect === 'function') {
            cr = container.getBoundingClientRect();
          }
        } catch (_) { cr = null; }
        if (cr && cr.width > 0 && cr.height > 0) {
          _transitionTo('stabilising');
          _stabilisationStartTs = (typeof Date !== 'undefined' && typeof Date.now === 'function')
            ? Date.now() : 0;
          _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED);
          _scheduleNextStabilisationFrame();
        }
        return;
      }
      if (_initialFitState === 'blocked_zero_container') {
        var crBlocked = null;
        try {
          if (container && typeof container.getBoundingClientRect === 'function') {
            crBlocked = container.getBoundingClientRect();
          }
        } catch (_) { crBlocked = null; }
        if (crBlocked && crBlocked.width > 0 && crBlocked.height > 0) {
          // Fresh start of initial stabilisation — reset stability
          // state, reset DOM attr to 'pending', re-arm the safety
          // cap from now, and schedule the first readiness frame.
          _lastFitSnapshot = null;
          _stableFrameCount = 0;
          _revealSnapshot  = null;
          _transitionTo('stabilising');
          try {
            container.setAttribute('data-initial-fit', 'pending');
          } catch (_) { /* swallow */ }
          _stabilisationStartTs = (typeof Date !== 'undefined' && typeof Date.now === 'function')
            ? Date.now() : 0;
          _armInitialSafetyCap();
          _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED);
          _scheduleNextStabilisationFrame();
        }
        return;
      }
      if (_initialFitState === 'stabilising') {
        return;
      }
      // Steady-state path (revealed / forced_reveal_*).
      try { cy.resize(); } catch (_) { /* swallow */ }
      _emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED);
      if (_tryPostRevealCorrection()) return;
      _emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE);
    }

    if (typeof window.ResizeObserver === 'function') {
      try {
        resizeObs = new window.ResizeObserver(_onContainerResize);
        resizeObs.observe(container);
      } catch (_) { resizeObs = null; }
    }

    // ── Initial state decision + safety cap ──
    //
    // D37ah-readiness-gated-initial-fit — Replace the D37af time-
    // gated stabilisation loop with a readiness-gated two-mode
    // lifecycle. Mount-time decision:
    //
    //   • Container has positive width AND height
    //       → transition to 'stabilising'; the readiness rAF loop
    //         takes over.
    //   • Container has zero width or height
    //       → remain in 'awaiting_container'; the ResizeObserver is
    //         responsible for promoting state once the container
    //         gains size. Emit DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED
    //         so dev tools can observe the deferred path.
    var initialContainerRect = null;
    try {
      if (container && typeof container.getBoundingClientRect === 'function') {
        initialContainerRect = container.getBoundingClientRect();
      }
    } catch (_) { initialContainerRect = null; }
    _stabilisationStartTs = (typeof Date !== 'undefined' && typeof Date.now === 'function')
      ? Date.now() : 0;
    if (initialContainerRect &&
        initialContainerRect.width > 0 &&
        initialContainerRect.height > 0) {
      _transitionTo('stabilising');
      _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED);
      _scheduleNextStabilisationFrame();
    } else {
      // Stay in 'awaiting_container'; ResizeObserver will promote.
      _emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED);
    }

    // D37ai-blocked-container-recovery — Arm the single external
    // safety-cap timer via the shared helper. The helper applies the
    // three-way SAFETY-CAP RULE at firing time and is also called
    // from the ResizeObserver blocked → stabilising recovery branch
    // to re-arm the cap from the recovery moment.
    _armInitialSafetyCap();

    return handle;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphCytoscapeEngine = {
    mount: mount,
    _constants: {
      CONTAINER_CLASS: CONTAINER_CLASS,
      CY_MOUNT_CLASS:  CY_MOUNT_CLASS,
    },
  };
})();
