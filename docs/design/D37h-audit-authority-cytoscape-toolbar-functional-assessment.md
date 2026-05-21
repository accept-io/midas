# D37h-audit — Authority Cytoscape Toolbar Functional Assessment

> **Status:** Read-only assessment. No MIDAS source, CSS, tests, or
> docs were modified. No commits, branches, fetches, or remote
> interaction.
>
> **Question being answered:** Why do the visible Authority Cytoscape
> toolbar controls (Zoom In, Zoom Out, Fit, Centre, Focus Mode, Zoom %,
> Zoom-to-selected, Reset view) not function against the Authority
> graph at runtime?

---

## 1. Executive summary

**Root cause (HIGH confidence):** The toolbar bridge's
`_pocActive()` gate reads a **retired** body class. D35f-retire-
transitional-renderer-debt removed the `body.cytoscape-poc-active`
class as the activation signal — nothing in the codebase adds it any
more. The bridge's existence-check, however, was never migrated. Every
toolbar click handler short-circuits on `if (!_pocActive()) return;`
and therefore never reaches the renderer's camera API.

Evidence:

- The class is documented as RETIRED at
  [authority-cytoscape-poc.js:122-132](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L122-L132):
  *"D35f-retire-transitional-renderer-debt — Body-class activation
  RETIRED. … The GraphViewport host (graph-viewport.js) sets [`data-
  active-renderer`] when `viewport.activateById('authority')`
  succeeds. The body class is no longer the source of truth."*
- Confirmation that nothing **adds** the class anywhere in JS: a
  full-tree grep for `classList.add('cytoscape-poc-active')` returns
  zero matches; every remaining reference is documentation, a removal
  comment, or a stale **read** like the toolbar's gate.
- The host's new activation signal is set at
  [graph-viewport.js:398-409](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L398-L409):
  `vp.setAttribute('data-active-renderer', rendererId)`.
- The toolbar gate still reads the old class at
  [authority-cytoscape-toolbar.js:64-67](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L64-L67):
  `return document.body.classList.contains('cytoscape-poc-active');`.

**Affected controls (immediate, on the click path):**

| Control | Click path |
|---|---|
| `#gmap-zoom-in` | capture handler `_onZoomIn` returns at line 84 |
| `#gmap-zoom-out` | `_onZoomOut` returns at line 93 |
| `#gmap-fit-button` | `_onFit` returns at line 102 |
| `#gmap-centre-button` | `_onCentre` returns at line 108 |
| `#gmap-zoom-selected-button` (D37h) | `_onZoomSelected` returns at line 129 |
| `#gmap-reset-view-button` (D37h) | `_onResetView` returns at line 142 |
| `#gmap-focus-toggle` | `_onFocusToggle` returns at line 121; legacy IIFE handler still toggles `body.gmap-focus-mode` so the chrome flip works, but the toolbar's belt-and-braces refit schedule is skipped |

**Affected adjacent paths:** the lens-aware Form/Records → List Mode
branch at [index.html:3269-3280](../../internal/httpapi/explorer/index.html#L3269-L3280)
uses the same retired class as its gate; List Mode is also broken.

**NOT directly affected by the `_pocActive()` bug:** the zoom-%
display, the zoom-to-selected disabled-state sync, and the double-tap
zoom — they bypass `_pocActive()` in different ways (see §6, §7, §10).
These have **separate** plumbing concerns that may produce a
*subjective* "doesn't work" perception, called out per-control below.

**Why D37h tests passed despite this:** the D37h test suite is
exclusively **asset-text pins**. It verifies that the function bodies
contain the expected substrings (e.g. `if (!_pocActive()) return;`,
`poc.zoomToSelected()`, `cy.on('dbltap', 'node'`). It does not
execute a browser, does not render the Authority lens, does not
simulate the renderer's body-class state, and does not assert that
the gate actually opens at runtime. See §12.

**Recommended next tranche:**
**D37h-fix-1 — Authority Cytoscape Toolbar Runtime Wiring Fix.**
Smallest safe change: migrate `_pocActive()` (and the same gate in the
index.html IIFE for List Mode) from `body.cytoscape-poc-active` to
`.midas-graph-viewport[data-active-renderer="authority"]`. Add a
**runtime/integration test** (or document a tight browser checklist)
so the next migration of this gate cannot pass tests but fail in
production. Do **not** proceed to D37i selection mode until D37h-fix-1
is fixed and browser-validated.

---

## 2. Toolbar DOM inventory

All eight controls are present inside `.gmap-camera-cluster` in
[index.html:521-532](../../internal/httpapi/explorer/index.html#L521-L532).
No duplicate IDs were observed elsewhere in the document.

| ID | Element | Default state | aria-label | title | Inside `.gmap-camera-cluster`? |
|---|---|---|---|---|---|
| `gmap-zoom-in` | `<button>` | enabled | `Zoom in` | `Zoom in` | ✅ |
| `gmap-zoom-out` | `<button>` | enabled | `Zoom out` | `Zoom out` | ✅ |
| `gmap-zoom-percent` (D37h) | `<span>` | `--%` placeholder, `role="status"`, `aria-live="polite"` | `Current zoom` | `Current zoom` | ✅ |
| `gmap-fit-button` | `<button>` | enabled | `Fit graph to view` | `Fit graph to view` | ✅ |
| `gmap-zoom-selected-button` (D37h) | `<button>` | **disabled, `aria-disabled="true"`** | `Zoom to selected` | `Zoom to selected` | ✅ |
| `gmap-centre-button` | `<button>` | enabled | `Centre on root` | `Centre on root` | ✅ |
| `gmap-reset-view-button` (D37h) | `<button>` | enabled | `Reset view` | `Reset view` | ✅ |
| `gmap-focus-toggle` | `<button>` | `aria-pressed="false"` | `Toggle focus mode` | `Toggle focus mode` | ✅ |

There is a separate `.gmap-mode-rail` two-button cluster (Pan / Select
mode) declared at
[index.html:515-520](../../internal/httpapi/explorer/index.html#L515-L520).
That is out of scope for D37h — those are the D37i candidate controls.

DOM presence is therefore **not the failure cause** for any control.

---

## 3. Script loading and wiring lifecycle

### 3.1 Script-tag order

[index.html:1708-1713](../../internal/httpapi/explorer/index.html#L1708-L1713):

```
<script src="…/vendor/cytoscape.min.js"></script>                  (1708)
<script src="…/authority/authority-cytoscape-poc.js"></script>     (1709)
<script src="…/authority/cytoscape-html-overlay.js"></script>      (1711)
<script src="…/authority/authority-cytoscape-toolbar.js"></script> (1713)
```

By the time the toolbar bridge IIFE runs at line 1713, the renderer
IIFE has already executed at line 1709 and registered
`window.MIDASExplorerGraph.cytoscapePoc`. The toolbar's `_poc()`
lookup at module-init time would therefore find a live object.

### 3.2 `wire()` invocation

[authority-cytoscape-toolbar.js:390-394](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L390-L394):

```js
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', wire);
} else {
  wire();
}
```

`wire()` runs once at DOMContentLoaded (the script lives in the body,
so the document is typically `loading` when the IIFE evaluates).

### 3.3 `wire()` body

[authority-cytoscape-toolbar.js:373-381](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L373-L381):

```js
function wire() {
  _bindCameraCluster();
  _bindBodyClassObserver();
  _bindWindowResize();
  _ensureSubscriptions();
}
```

All four steps are idempotent. The `_bindCameraCluster` loop at
[lines 207-231](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L207-L231)
iterates a fixed list of seven IDs and attaches a capture-phase click
listener to each. D37h added `gmap-zoom-selected-button` and
`gmap-reset-view-button` to this list at
[lines 217-218](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L217-L218).

**No race conditions observed.** Every control is present in the DOM
before `wire()` runs; PoC is registered before the toolbar IIFE
evaluates; `_ensureSubscriptions()` is idempotent and re-runs on every
body-class mutation if the first attempt was a no-op.

---

## 4. `_pocActive()` / active-renderer gate assessment

### 4.1 What the gate checks

[authority-cytoscape-toolbar.js:59-67](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L59-L67):

```js
// _pocActive reads the body class set by the PoC IIFE
// (authority-cytoscape-poc.js:104). This is the canonical "is the
// Cytoscape PoC the active Authority renderer?" check. It stays
// accurate even when the PoC is mid-unmount (the class is cleared
// in _uninstallPoc).
function _pocActive() {
  if (typeof document === 'undefined' || !document.body) return false;
  return document.body.classList.contains('cytoscape-poc-active');
}
```

The comment cites `authority-cytoscape-poc.js:104` as the canonical
add-site. The renderer file at that line range is now the *retirement
documentation*, not an add.

### 4.2 What the runtime actually does

The renderer at
[authority-cytoscape-poc.js:122-132](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L122-L132)
explicitly documents the retirement:

> *"D35f-retire-transitional-renderer-debt — Body-class activation
> RETIRED. Pre-D35f the IIFE added `body.cytoscape-poc-active`
> unconditionally on `?cytoscape=1` … D35f moves both rules onto
> `.midas-graph-viewport[data-active-renderer="authority"]` … The
> GraphViewport host (graph-viewport.js) sets this attribute when
> `viewport.activateById('authority')` succeeds. The body class is
> no longer the source of truth."*

The renderer's teardown path at
[authority-cytoscape-poc.js:1838-1845](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1838-L1845)
confirms:

> *"Body-class removal RETIRED. The host's `viewport.deactivate()` …
> clears the `data-active-renderer="authority"` attribute on
> `.midas-graph-viewport` … which is now the sole source of strategic
> renderer-state CSS keying. The pre-D35f `body.cytoscape-poc-active`
> flip is gone."*

Nothing else in
[`internal/httpapi/explorer/assets/js/`](../../internal/httpapi/explorer/assets/js/)
adds the class — a full grep across the directory returns the
following: the renderer's retirement comments, the toolbar's stale
read, the index.html inline IIFE that also reads it for List Mode
gating, and the css spike file (comment reference only). **No JS
performs `classList.add('cytoscape-poc-active')`.**

### 4.3 Consequence

**`_pocActive()` always returns `false` at runtime.** Every handler in
the bridge that begins with `if (!_pocActive()) return;` is therefore
a no-op:

| Handler | Line | Effect when clicked |
|---|---|---|
| `_onZoomIn` | [83-90](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L83-L90) | Returns; legacy bubble handler at [index.html:2641](../../internal/httpapi/explorer/index.html#L2641) drives the *hidden* SVG canvas (zero visible effect) |
| `_onZoomOut` | [92-99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L92-L99) | Returns; legacy handler at [index.html:2642](../../internal/httpapi/explorer/index.html#L2642) drives hidden SVG |
| `_onFit` | [101-105](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L101-L105) | Returns; legacy handler at [index.html:2779-2781](../../internal/httpapi/explorer/index.html#L2779-L2781) drives hidden SVG |
| `_onCentre` | [107-121](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L107-L121) | Returns; legacy handler at [index.html:2663-2670](../../internal/httpapi/explorer/index.html#L2663-L2670) drives hidden SVG |
| `_onZoomSelected` (D37h) | [128-134](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L128-L134) | Returns; **no legacy handler exists** → nothing happens at all |
| `_onResetView` (D37h) | [141-147](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L141-L147) | Returns; **no legacy handler exists** → nothing happens at all |
| `_onFocusToggle` | [120-127](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L120-L127) (note: no stopProp) | Returns; legacy handler still flips `body.gmap-focus-mode` so the *chrome* flip works; the toolbar's `_scheduleRefit(DRAWER_TRANSITION_MS)` is skipped → Cytoscape may still resize via the body-class observer's separate refit path (see §11) |

This is **the primary failure cause** for every existing click handler.

---

## 5. Public Cytoscape API surface assessment

The PoC public surface is registered at
[authority-cytoscape-poc.js:3643-3722](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3643-L3722).
The D37h additions are present at lines 3666-3686.

| Toolbar call | Expected public function | Exists? | Calls active `_cy`? | Risk |
|---|---|---|---|---|
| `poc.zoomBy(factor)` | `cytoscapePoc.zoomBy` (line 3660) | ✅ | ✅ via module-scope `_cy` | Reached only if `_pocActive()` true — **blocked by gate** |
| `poc.fit()` | `cytoscapePoc.fit` (line 3659) | ✅ | ✅ | **blocked by gate** |
| `poc.centerOnRoot()` | `cytoscapePoc.centerOnRoot` (line 3661) | ✅ | ✅ | **blocked by gate** |
| `poc.zoomToSelected()` | `cytoscapePoc.zoomToSelected` (line 3670) | ✅ | ✅ — reads `cy.elements(':selected')` | **blocked by gate** |
| `poc.zoomToNode(id)` | `cytoscapePoc.zoomToNode` (line 3671) | ✅ | ✅ via `_cy.$id(id)` | **not bound to a button**; available for double-tap |
| `poc.resetView()` | `cytoscapePoc.resetView` (line 3677) | ✅ | ✅ delegates to `_fitToAvailableCanvas` | **blocked by gate** |
| `poc.getZoomPercent()` | `cytoscapePoc.getZoomPercent` (line 3678) | ✅ | ✅ returns `Math.round(_cy.zoom()*100)` or null | not blocked by `_pocActive()` |
| `poc.onViewportChanged(h)` | `cytoscapePoc.onViewportChanged` (line 3679) | ✅ | ✅ registers handler; binds to current `_cy` when present; rebinds on each new cy via `_attachExternalHandlersToCy` | not blocked by `_pocActive()` |
| `poc.onSelectionChanged(h)` | `cytoscapePoc.onSelectionChanged` (line 3680) | ✅ | ✅ same shape | not blocked by `_pocActive()` |
| `poc.isReady()` | `cytoscapePoc.isReady` (line 3669) | ✅ | returns `!!_cy` | **declared but not called anywhere in the toolbar** |
| `poc.ZOOM_STEP_FACTOR` | constant (line 3667) | ✅ | n/a | n/a |

**All API symbols exist under the expected names.** The naming
contract is intact. The failure is upstream of every API call: the
gate returns false before any call site is reached.

---

## 6. Existing control functional traces

### 6.1 `#gmap-zoom-in`

| Step | Result |
|---|---|
| DOM element | Present at [index.html:522](../../internal/httpapi/explorer/index.html#L522) |
| Capture listener attached | ✅ via `_bindCameraCluster` |
| Handler `_onZoomIn` runs in capture phase | ✅ |
| `_pocActive()` check at [line 84](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L84) | **❌ returns false → handler returns** |
| `stopImmediatePropagation` reached? | ❌ never reached |
| Legacy bubble handler at [index.html:2641](../../internal/httpapi/explorer/index.html#L2641) | ✅ runs → `setGmapZoom(...)` on the *hidden* SVG canvas |
| Result visible to user | **None** — Cytoscape graph is unchanged |

### 6.2 `#gmap-zoom-out`

Identical pattern. Bridge returns at [line 93](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L93);
legacy handler at [index.html:2642](../../internal/httpapi/explorer/index.html#L2642).

### 6.3 `#gmap-fit-button`

Bridge returns at [line 102](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L102);
legacy handler at [index.html:2779-2781](../../internal/httpapi/explorer/index.html#L2779-L2781)
calls `fitGmapToBounds()` on the SVG canvas.

### 6.4 `#gmap-centre-button`

Bridge returns at [line 108](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L108);
legacy handler at [index.html:2663-2670](../../internal/httpapi/explorer/index.html#L2663-L2670)
calls `focusGmapOnRoot()` on the SVG canvas. The D37h preservation-of-
legacy-behaviour outcome is moot because the legacy behaviour is on
the hidden SVG, not Cytoscape.

### 6.5 `#gmap-focus-toggle`

Bridge returns at [line 121](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L121).
This handler does **not** call `_stop(ev)`, so the legacy bubble-phase
handler still runs and toggles `body.gmap-focus-mode`. CSS keyed on
that class hides shell chrome, so the *visual* focus-mode toggle does
work. What's lost is the bridge's belt-and-braces
`_scheduleRefit(DRAWER_TRANSITION_MS)` so Cytoscape may not resize
optimally after the chrome change. The body-class observer at
[lines 322-355](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L322-L355)
*does* watch for `gmap-focus-mode` changes, but its refit also gates
on `_pocActive()` at [line 337](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L337)
— so this fallback is also blocked.

**Net effect:** focus mode visually engages, but the Cytoscape graph
does not re-fit until some other event (e.g. window resize) triggers
a refit through an unblocked path.

---

## 7. New D37h control functional traces

### 7.1 `#gmap-zoom-percent` (display)

| Step | Result |
|---|---|
| DOM element | Present at [index.html:524](../../internal/httpapi/explorer/index.html#L524) |
| Initial render | `--%` placeholder in markup |
| `_renderZoomPercent` called in `_ensureSubscriptions` | ✅ at [line 310](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L310) |
| `_renderZoomPercent` gates on `_pocActive()` | **❌ — no gate, reads `poc.getZoomPercent()` directly** |
| `poc.getZoomPercent()` returns null when `_cy` missing | ✅ at [line 264](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L264) |
| Subscription installed | `_ensureViewportSubscription` registers handler with renderer; renderer's `_onViewportChanged` adds to module-scope `_viewportChangeHandlers` array; if `_cy` exists, binds immediately; otherwise binds when next `_cy` is created via `_attachExternalHandlersToCy(_cy)` at [authority-cytoscape-poc.js:3312-3313](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3312-L3313) |
| Subscription fires on `cy.on('zoom pan resize')` | ✅ in principle |

**Plausible runtime trajectory:** On initial DOMContentLoaded, `_cy`
is null because no lens has activated yet. `_ensureSubscriptions`
registers the handler in the renderer's registry but the binding to
`_cy` doesn't happen. The badge stays `--%`. When the user navigates
to the Authority lens and the renderer creates `_cy`,
`_wireInteractions` ends with `_attachExternalHandlersToCy(_cy)` which
binds the registered handler. The initial post-init `_settleFit` at
[authority-cytoscape-poc.js:3465-3470](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3465-L3470)
calls `cy.resize()` and `_fitToAvailableCanvas(cy)`, which fire
`zoom`/`pan`/`resize` events on the new `_cy`. The handler fires;
`_renderZoomPercent` calls `poc.getZoomPercent()` which now returns
a number; the badge updates.

**Expected user-visible behaviour:** badge should populate to a real
percentage on Authority activation and update on user pan/zoom.

**If the user reports it stays at `--%`:** this would suggest either
(a) `_attachExternalHandlersToCy(_cy)` is not being reached, (b) the
subscription was installed *before* the body-class observer's first
mutation cleared the cache and never re-installed, or (c) timing —
the subscription handler is registered after the initial fit fires.
**Requires browser validation** to confirm.

### 7.2 `#gmap-zoom-selected-button`

| Step | Result |
|---|---|
| DOM element | Present at [index.html:527](../../internal/httpapi/explorer/index.html#L527), starts with `disabled` + `aria-disabled="true"` |
| Capture listener attached | ✅ at [line 217-218](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L217-L218) |
| Disabled-state sync `_syncZoomSelectedEnabled` | At [line 267-286](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L267-L286), does **not** gate on `_pocActive()`; reads `poc.getCy()` and `cy.elements(':selected').length` |
| Selection subscription | `onSelectionChanged` installed via `_ensureSelectionSubscription` at [line 296-302](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L296-L302); renderer's `_onSelectionChanged` adds handler to `_selectionChangeHandlers` registry; binds to live `_cy` immediately if present; otherwise via `_attachExternalHandlersToCy` on next mount |
| Tap handler in renderer fires `select` | ✅ — [authority-cytoscape-poc.js:3266-3268](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3266-L3268) calls `node.select()` |
| Disabled state should transition disabled → enabled when user taps a node | ✅ — provided the subscription was bound to the live `_cy` |
| **Click handler when enabled** | **❌ — `_onZoomSelected` returns at [line 129](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L129) because `_pocActive()` is false** |

**Net effect:** even if the disabled state correctly transitions
(plausible from static analysis), clicking the button does nothing —
the gate blocks the call to `poc.zoomToSelected()` and there is no
legacy fallback handler.

### 7.3 `#gmap-reset-view-button`

| Step | Result |
|---|---|
| DOM element | Present at [index.html:529](../../internal/httpapi/explorer/index.html#L529) |
| Capture listener attached | ✅ |
| Click handler | **❌ — `_onResetView` returns at [line 142](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L142)**; no legacy handler exists |

Same root cause as zoom-to-selected: gate blocks `poc.resetView()`.

### 7.4 Double-click node zoom (`cy.on('dbltap', 'node', …)`)

The dbltap binding lives **inside the renderer's `_wireInteractions`**
at [authority-cytoscape-poc.js:3305-3320](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3305-L3320),
not in the toolbar bridge. The renderer binds it directly on `_cy`
when the cy is created. It does **not** route through `_pocActive()`.
Cards are `pointer-events: none` per
[authority-cytoscape-poc.css:103](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L103)
so the underlying Cytoscape canvas receives the click pair.

**Static analysis says this should work in the browser.** If the user
reports it doesn't, candidates are:

- Card pointer-events override somewhere (search of the css shows
  every card / overlay rule is `pointer-events: none`; no override
  found).
- Double-tap timing: Cytoscape's `multiClickDebounceTime` default;
  rapid single-tap → second tap sequence.
- HTML card stacking blocking the canvas (would block single-tap
  selection too — observable test).

**Requires browser validation.**

---

## 8. Selection / disabled-state assessment

`#gmap-zoom-selected-button` starts disabled at
[index.html:527](../../internal/httpapi/explorer/index.html#L527).
Its enablement chain:

1. User taps an Authority node.
2. Renderer's `tap`-on-`node` handler at
   [authority-cytoscape-poc.js:3264-3290](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3264-L3290)
   runs: `_cy.elements().unselect(); node.select(); _focusNode(node); _emphasiseRootPath(node);` then dispatches the inspector hook.
3. `node.select()` fires the `select` event on the node.
4. Renderer's external-handler registry (set up by
   `_attachExternalHandlersToCy(_cy)` at line 3312-3313) routes the
   `select`/`unselect` events to subscribed external handlers.
5. The toolbar's subscription handler at
   [authority-cytoscape-toolbar.js:300](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L300)
   calls `_syncZoomSelectedEnabled()`.
6. `_syncZoomSelectedEnabled` reads `cy.elements(':selected').length`,
   sees 1, removes `disabled`.

**Static analysis:** this should work, *provided the subscription is
bound to the live `_cy`*.

Timing concern: `_ensureSelectionSubscription` runs in `wire()` which
runs at DOMContentLoaded. At that moment the active lens may not yet
be Authority, so the renderer hasn't created `_cy` yet. The
subscription is registered in the renderer's registry but binding to
cy is deferred. When the user navigates to Authority and `_cy` is
created, `_attachExternalHandlersToCy(_cy)` binds the registered
handler to the new cy.

If the user navigates to Authority but the disabled state never
transitions:

- `_attachExternalHandlersToCy(_cy)` may not have run (e.g. `_cy`
  initialised through a different path).
- The renderer's `_viewportChangeHandlers`/`_selectionChangeHandlers`
  arrays may have been re-created (they are declared at
  [authority-cytoscape-poc.js:1117-1118](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1117-L1118),
  module-scope so survive re-mounts).
- Multiple subscriptions could install if `_ensureSubscriptions` ran
  before `_viewportSubscribed`/`_selectionSubscribed` was set —
  defensive flag is checked first ([line 289](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L289),
  [line 297](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L297))
  so duplicate subscriptions are unlikely.

**Requires browser validation** to confirm whether the disabled→enabled
transition actually happens. The click handler is broken regardless
(see §7.2).

---

## 9. Zoom-percentage update assessment

Detailed in §7.1. Summary:

- `_renderZoomPercent` does **not** gate on `_pocActive()`. It reads
  `poc.getZoomPercent()` directly which returns `Math.round(cy.zoom() * 100)`
  or `null` if `_cy` is missing.
- The badge is initialised to `--%` in markup; `_ensureSubscriptions`
  calls `_renderZoomPercent` synchronously at wire-time (returns
  `--%` because `_cy` is null) and registers an `onViewportChanged`
  subscription that updates the badge on subsequent `zoom`/`pan`/
  `resize` events.
- Subscription handler is registered into the renderer's
  module-scope `_viewportChangeHandlers` registry; binding to a
  live `_cy` happens via `_attachExternalHandlersToCy(_cy)` at the
  end of `_wireInteractions`.

**Static analysis:** badge should populate after Authority activation.
Initial fit / settle cycle calls `cy.resize()` and `_fitToAvailableCanvas`
which fires multiple zoom/pan events.

**Requires browser validation.**

---

## 10. Double-click zoom assessment

Detailed in §7.4. Summary:

- Bound directly on the renderer-side `_cy` via
  `_cy.on('dbltap', 'node', ...)` at
  [authority-cytoscape-poc.js:3305-3320](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3305-L3320).
- Independent of toolbar bridge and `_pocActive()`.
- Cards are `pointer-events: none`; Cytoscape receives the click
  pair.

**Static analysis:** should work. **Requires browser validation.**

---

## 11. Overlay / pointer-event assessment

| Element | `pointer-events` | Evidence |
|---|---|---|
| HTML overlay layer (`.cytoscape-poc-html-overlay`) | `none` | [authority-cytoscape-poc.css:103](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L103) |
| HTML card (`.cytoscape-poc-html-card`) | `none` | [authority-cytoscape-poc.css:124](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L124) |
| Toolbar buttons | default (auto) — clickable | governance-map.css `.gmap-camera-cluster button` rules |
| Zoom % badge | `cursor: default` + `user-select: none`; pointer-events default — not blocking | [governance-map.css](../../internal/httpapi/explorer/assets/css/governance-map.css) |

The HTML overlay sits at `z-index: 5` on `.cytoscape-poc-html-overlay`
via [authority-cytoscape-poc.css:104](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L104).
The toolbar (`.gmap-camera-cluster`) is at `z-index: 5` per
[governance-map.css:223](../../internal/httpapi/explorer/assets/css/governance-map.css#L223).
Same z-index but the toolbar is later in document source order, so it
should win the stacking battle. The toolbar buttons are clickable
(user reports they "are present in the UI") so this is not a barrier
in practice.

**No pointer-event interference detected** as a cause of the toolbar
failure. The graph is also reachable through HTML cards because of
pointer-events:none cascade.

---

## 12. Test adequacy gap analysis

The D37h test suite at
[explorer_d37h_test.go](../../internal/httpapi/explorer_d37h_test.go)
is **entirely asset-text pins**. Every test verifies that source
files contain expected substrings; none execute JavaScript.

### 12.1 What the existing tests do verify

- DOM ID presence (matters)
- aria-label / title presence (matters)
- Source contains expected function-body fragments
- API symbols exist on the public surface
- CSS rule presence in the correct stylesheet
- Negative pins (no `cy.reset()` call; no HTML card DOM writes from
  the bridge; no node-kind icon class names in the camera cluster)

### 12.2 What they do NOT verify

- That the runtime gate `_pocActive()` ever returns true at runtime.
  (The retired body class would have been caught by a runtime test or
  a paired asset-text pin that confirmed the gate was rewritten to
  read `data-active-renderer="authority"`.)
- That clicking a toolbar button reaches the renderer API.
- That the renderer's `_cy` is created and bound to subscribers in
  response to lens activation.
- That `getZoomPercent()` returns a number after activation.
- That `cy.on('dbltap', 'node', …)` actually fires in the browser.
- That HTML cards genuinely cascade pointer events to the underlying
  Cytoscape canvas.
- That the disabled-state of `#gmap-zoom-selected-button` transitions
  in response to a `select` event.
- That the existing `_pocActive()` gate is even *compatible* with the
  active-renderer signal currently emitted by the host.

### 12.3 The root contradiction

The test `TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass` at
[explorer_authority_cytoscape_poc_test.go:303-341](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L303-L341)
**knows** the activation signal is `.midas-graph-viewport[data-active-renderer="authority"]`
and pins that **every CSS rule** in the renderer stylesheet is scoped
to that selector. It is therefore **established in the test suite**
that the body-class gate has been retired in favour of the renderer-
identity attribute. But there is **no companion test** that pins the
*toolbar bridge gate* to the same signal. Asset-text pins on the
toolbar bridge body don't catch this because the bridge still uses
the *retired* function name and string — both are syntactically
correct, they just describe behaviour that no longer exists.

### 12.4 Recommendations for future tests (deferred to D37h-fix-1)

- A **runtime/integration test** that loads `/explorer`, activates
  the Authority lens, asserts the renderer is mounted, then
  dispatches a synthetic click on `#gmap-zoom-in` and asserts
  `cy.zoom()` increased. A Playwright/Cypress browser test is the
  natural fit.
- A **static asset-text test** that pins the gate to the *current*
  activation signal — e.g. confirm `_pocActive()` body contains
  `'data-active-renderer'` and references `.midas-graph-viewport`.
- A **paired-rename guard test** that confirms `body.cytoscape-poc-active`
  appears in EXECUTABLE JS only in commented-out/retired contexts,
  not in any condition.
- A **subscription-rebind test** that asserts `_attachExternalHandlersToCy`
  is called inside `_wireInteractions` (already pinned by D37h test
  `ViewportSubscriptionUsesCytoscapeEvents`), but extend to assert
  that the registries are iterated and `cy.on(...)` is called for each
  registered handler.

---

## 13. Ranked likely root causes

| # | Cause | Confidence | Affected controls | Why tests missed it |
|---|---|---|---|---|
| **1** | **`_pocActive()` reads the retired `body.cytoscape-poc-active` class; the active signal is now `.midas-graph-viewport[data-active-renderer="authority"]`** | **HIGH** | Zoom In, Zoom Out, Fit, Centre, Zoom-to-selected, Reset view, Focus-toggle refit, List-mode toggle | Asset-text pins matched the (stale) function body verbatim; no runtime test confirms the gate returns true on the real activation signal |
| 2 | Body-class observer's refit branch also gates on `_pocActive()` → focus-mode refit + drawer-mutation refit silently broken since D35f | HIGH | refit timing only (visual) | Same — no runtime test |
| 3 | Zoom-to-selected and Reset view have no legacy fallback handler (unlike existing buttons); user perceives them as "totally dead" while the existing buttons may have unobserved hidden-SVG side effects | HIGH | Zoom-to-selected, Reset view | D37h tests intentionally do not bind any legacy fallback |
| 4 | Inline `pocActive` check at [index.html:3269](../../internal/httpapi/explorer/index.html#L3269) gates Form/Records → List-Mode toggle on the same retired class; same root cause | HIGH | List-mode toggle (out of D37h scope but pertinent to the gate-migration fix) | No runtime test |
| 5 | Stale comment at [authority-cytoscape-toolbar.js:59-63](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L59-L63) still cites the renderer line that added the class, masking the regression in code review | MEDIUM | n/a (process risk) | n/a |
| 6 | Possible timing issue: subscription handlers registered before `_cy` is created depend on `_attachExternalHandlersToCy(_cy)` running on each fresh `_cy`. Static analysis confirms it runs at the end of `_wireInteractions`, but a regression here would silently break zoom% + disabled-state. | LOW | Zoom%, zoom-to-selected disabled-state | D37h asset-text test pins the `_attachExternalHandlersToCy(_cy)` call but not its execution |
| 7 | The badge default text `--%` in markup may appear "broken" to a user who navigates between lenses and never sees it update, even though the underlying subscription is wired. Unlikely to be the user-reported issue. | LOW | Zoom% only | Static |
| 8 | Pointer-events / overlay stacking blocking double-tap. Static analysis says no (all overlays are pointer-events:none). | LOW | dbltap | Requires browser validation |
| 9 | Cytoscape `multiClickDebounceTime` default could mishandle rapid double-clicks. | LOW | dbltap | Requires browser validation |

**Causes 1–4 are the same underlying defect** (the migrated gate was
missed in two places: the bridge `_pocActive()` and the inline IIFE
in index.html). Fixing the gate fixes Causes 1–4 in one shot.

---

## 14. Recommended narrow fix tranche

**Name:** **D37h-fix-1 — Authority Cytoscape Toolbar Runtime Wiring Fix.**

### 14.1 Scope (in)

1. **Migrate `_pocActive()` in
   [authority-cytoscape-toolbar.js:64-67](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L64-L67)**:
   instead of reading `body.classList.contains('cytoscape-poc-active')`,
   read the host-owned signal: the presence of
   `data-active-renderer="authority"` on `.midas-graph-viewport`. The
   simplest probe is
   `document.querySelector('.midas-graph-viewport[data-active-renderer="authority"]') != null`
   — or via the host's public API `viewport.getActiveRendererId() === 'authority'`
   (already available at
   [graph-viewport.js:542-544](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L542-L544)).
   Prefer the public API for cleanness; fall back to the DOM probe
   for resilience.
2. **Update the leading docstring at
   [authority-cytoscape-toolbar.js:1-44](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L1-L44)**
   and the helper comment at lines 59-63 to reference the new signal
   and the D35f retirement.
3. **Migrate the inline `pocActive` check at
   [index.html:3269](../../internal/httpapi/explorer/index.html#L3269)**
   to the same signal so the Form/Records → List Mode toggle
   recovers in lockstep.
4. **Loosen the body-class observer's refit gate** so it triggers on
   *any* renderer-identity change. The observer currently watches
   only `body[class]`; the active-renderer attribute lives on the
   viewport element, not body. **Either** (a) add a second
   MutationObserver on `.midas-graph-viewport[attributes]` watching
   `data-active-renderer`, OR (b) leave the existing observer in
   place but rely on `_pocActive()` reading the new signal — the
   refit then naturally runs on the next body-class mutation
   (`gmap-focus-mode`, `gmap-inspector-collapsed`).

### 14.2 Tests to add / update

- New asset-text pin: `_pocActive()` body must contain
  `data-active-renderer` and either `'.midas-graph-viewport'` or
  `viewport.getActiveRendererId()`.
- New asset-text pin: index.html inline `pocActive` check must use
  the same signal.
- **New negative pin**: `_pocActive()` body must NOT contain
  `classList.contains('cytoscape-poc-active')`.
- **New runtime / browser test** (recommended Playwright): activate
  Authority, click `#gmap-zoom-in`, assert `window.MIDASExplorerGraph.cytoscapePoc.getCy().zoom()`
  increased; same for fit / centre / reset / zoom-to-selected.
- D37h tests at `explorer_d37h_test.go` continue to pass without
  change.

### 14.3 Browser validation checklist (post-fix)

1. Authority lens opens; HTML cards visible.
2. Zoom In increases `cy.zoom()` and the visible Cytoscape graph
   scales up. Cards stay aligned.
3. Zoom Out decreases zoom; cards stay aligned.
4. Fit re-fits to safe-area padding; cards visible.
5. Centre (legacy preserved behaviour) pans the Cytoscape graph to
   the root.
6. Focus toggle still toggles `body.gmap-focus-mode`; Cytoscape
   re-fits after the chrome transition.
7. Zoom-% badge populates after activation and updates on user pan/
   zoom.
8. Tap a node → `#gmap-zoom-selected-button` transitions from
   disabled to enabled; clicking it zooms to the node.
9. Reset view returns to a safe-area-aware whole-graph fit.
10. Double-click a card → zooms to that underlying Cytoscape node.
11. List Mode (Form/Records button) toggles correctly.
12. Existing right-drawer / diagnostics / posture panel populates as
    before.

### 14.4 Scope (out)

- D37i (selection mode, multi-select, box selection, hide-show,
  bulk actions) — **do not start D37i until D37h-fix-1 is fixed and
  browser-validated.**
- Centre-button semantic redesign (still legacy `centerOnRoot`).
- New controls beyond the D37h set.
- Cytoscape extensions.
- Backend / projection changes.

### 14.5 Risk

Very low. The fix is a one-line replacement plus a docstring update;
the resulting behaviour is exactly what the D37h tests *intended* to
pin. The runtime/integration test is the highest-value addition for
preventing the next migration of this gate from regressing the same
way.

---

## 15. Evidence appendix

### 15.1 Toolbar bridge

| Claim | Citation |
|---|---|
| `_pocActive()` reads retired class | [authority-cytoscape-toolbar.js:64-67](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L64-L67) |
| Bridge docstring still cites old class | [authority-cytoscape-toolbar.js:1-44](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L1-L44) |
| `_onZoomIn` gates on `_pocActive()` | [authority-cytoscape-toolbar.js:83-90](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L83-L90) |
| `_onZoomOut` gates on `_pocActive()` | [authority-cytoscape-toolbar.js:92-99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L92-L99) |
| `_onFit` gates on `_pocActive()` | [authority-cytoscape-toolbar.js:101-105](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L101-L105) |
| `_onCentre` gates on `_pocActive()` | [authority-cytoscape-toolbar.js:107-121](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L107-L121) |
| `_onZoomSelected` (D37h) gates on `_pocActive()` | [authority-cytoscape-toolbar.js:128-134](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L128-L134) |
| `_onResetView` (D37h) gates on `_pocActive()` | [authority-cytoscape-toolbar.js:141-147](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L141-L147) |
| `_onFocusToggle` gates on `_pocActive()` | [authority-cytoscape-toolbar.js:120-127](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L120-L127) |
| Capture-phase listener binding | [authority-cytoscape-toolbar.js:207-231](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L207-L231) |
| Subscription helpers (D37h) | [authority-cytoscape-toolbar.js:288-312](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L288-L312) |
| Body-class observer + `_pocActive` refit gate | [authority-cytoscape-toolbar.js:322-355](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L322-L355) |
| `wire()` invocation at DOMContentLoaded | [authority-cytoscape-toolbar.js:390-394](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L390-L394) |
| Public surface | [authority-cytoscape-toolbar.js:396-406](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L396-L406) |

### 15.2 Renderer

| Claim | Citation |
|---|---|
| D35f-retire-transitional-renderer-debt commentary | [authority-cytoscape-poc.js:122-132](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L122-L132) |
| D35f body-class removal | [authority-cytoscape-poc.js:1838-1845](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1838-L1845) |
| Tap handler: `node.select()` | [authority-cytoscape-poc.js:3264-3290](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3264-L3290) |
| `cy.on('dbltap', 'node', …)` | [authority-cytoscape-poc.js:3305-3320](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3305-L3320) |
| `_attachExternalHandlersToCy(_cy)` end-of-wire | [authority-cytoscape-poc.js:3312-3313](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3312-L3313) |
| D37h API: `_zoomToNode`, `_zoomToSelected`, `_resetView`, `_getZoomPercent` | [authority-cytoscape-poc.js:1119-1212](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1119-L1212) |
| D37h API: registries + subscribers | [authority-cytoscape-poc.js:1213-1252](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1213-L1252) |
| Public surface (incl. D37h additions) | [authority-cytoscape-poc.js:3643-3722](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3643-L3722) |

### 15.3 Host (graph-viewport.js)

| Claim | Citation |
|---|---|
| `data-active-renderer` as the new signal | [graph-viewport.js:384-398](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L384-L398) |
| `_setActiveRendererAttribute` | [graph-viewport.js:400-409](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L400-L409) |
| `getActiveRendererId()` | [graph-viewport.js:542-544](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L542-L544) |

### 15.4 Markup (index.html)

| Claim | Citation |
|---|---|
| `.gmap-camera-cluster` wrapper | [index.html:521](../../internal/httpapi/explorer/index.html#L521) |
| All 8 controls inside the cluster | [index.html:522-530](../../internal/httpapi/explorer/index.html#L522-L530) |
| Script order: poc → toolbar | [index.html:1709, 1713](../../internal/httpapi/explorer/index.html#L1709) |
| Legacy IIFE zoom-in/out bindings | [index.html:2641-2643](../../internal/httpapi/explorer/index.html#L2641-L2643) |
| Legacy IIFE centre binding | [index.html:2663-2670](../../internal/httpapi/explorer/index.html#L2663-L2670) |
| Legacy IIFE fit binding | [index.html:2779-2781](../../internal/httpapi/explorer/index.html#L2779-L2781) |
| Inline `pocActive` check on List Mode toggle | [index.html:3269](../../internal/httpapi/explorer/index.html#L3269) |

### 15.5 CSS

| Claim | Citation |
|---|---|
| Overlay `pointer-events: none` | [authority-cytoscape-poc.css:103](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L103) |
| Card `pointer-events: none` | [authority-cytoscape-poc.css:124](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L124) |
| Camera cluster z-index / position | [governance-map.css:219-232](../../internal/httpapi/explorer/assets/css/governance-map.css#L219-L232) |
| Camera-cluster button shared rules | [governance-map.css:238-268](../../internal/httpapi/explorer/assets/css/governance-map.css#L238-L268) |
| D37h badge rule | [governance-map.css:280-296](../../internal/httpapi/explorer/assets/css/governance-map.css#L280-L296) |

### 15.6 Tests

| Claim | Citation |
|---|---|
| D37h test suite (asset-text pins only) | [explorer_d37h_test.go](../../internal/httpapi/explorer_d37h_test.go) |
| Renderer CSS scoping pin (confirms migrated signal) | [explorer_authority_cytoscape_poc_test.go:303-341](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L303-L341) |
| D32b-impl-2a `ToolbarNotHiddenInFocusMode` (only pins CSS hide-rule, not click wiring) | [explorer_d32a_test.go:3746](../../internal/httpapi/explorer_d32a_test.go#L3746) |
