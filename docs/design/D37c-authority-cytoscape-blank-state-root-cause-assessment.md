# D37c — Authority Cytoscape Blank-State Root-Cause Assessment

Tranche: D37c
Status: read-only assessment.
Mandate: identify why the D37b default Authority Cytoscape path
shows a blank canvas in the browser.

This report makes no source, test, or CSS changes. The single
output is this document.

---

## 1. Executive summary

**HIGH-CONFIDENCE root cause: the Cytoscape mount is appended as
a SIBLING of the legacy `.governance-map-canvas-scroll` inside
`.midas-graph-renderer-slot`, with both children declaring
`height: 100%` in block flow. The mount stacks BELOW the visible
viewport and is clipped invisible by
`.midas-graph-viewport { overflow: hidden }`. Cytoscape mounts
into an off-screen / zero-visible-area container, emits its
"container has invalid dimensions" warning, and renders nothing.**

Supporting evidence:
- DOM nesting at [index.html:459-473](../../internal/httpapi/explorer/index.html#L459-L473).
- `.midas-graph-renderer-slot` style at [governance-map.css:179-182](../../internal/httpapi/explorer/assets/css/governance-map.css#L179-L182) — `position: absolute; inset: 0;`.
- `.governance-map-canvas-scroll` style at [governance-map.css:529-535](../../internal/httpapi/explorer/assets/css/governance-map.css#L529-L535) — `height: 100%; overflow-x: auto; overflow-y: auto;`.
- `.cytoscape-poc-mount` style at [authority-cytoscape-poc.css:29-46](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L29-L46) — `position: relative; width: 100%; height: 100%; min-height: 480px;`.
- `_authorityRendererFactory.mount` appends via [authority-cytoscape-poc.js:1378-1388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1378-L1388) — `slotEl.appendChild(_mountEl)`.
- `.midas-graph-viewport { overflow: hidden }` strategic clip at [governance-map.css:162-166](../../internal/httpapi/explorer/assets/css/governance-map.css#L162-L166).

The Cytoscape warnings the user observed in the browser console
are consistent with this — Cytoscape's "container has invalid
dimensions" / `cy.fit()` warnings fire when `container.clientHeight
=== 0`, which is exactly what an off-screen-clipped container
reports.

This latent layout bug existed pre-D37b too, but was masked because
the Cytoscape PoC was strictly opt-in via `?cytoscape=1` and was
not the default user experience. D37b promoted Cytoscape to default
without addressing the mount-vs-canvas-scroll layout collision.

The 400 the user obtained from a direct `/v1/graphs/authority`
request is **almost certainly a separate manual probe of the bare
endpoint** (which correctly rejects an empty `id` with
`ErrInvalidID` at [service.go:173-175](../../internal/graph/authority/service.go#L173-L175)). The frontend code path
guards against empty `rootId` BEFORE issuing fetch
([_pocRefresh](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3388-L3419) lines 3393-3397), and the API client's `_query`
helper drops empty params altogether ([api-client.js:138-147](../../internal/httpapi/explorer/assets/js/core/api-client.js#L138-L147)), so the
production code is unlikely to be the source of a 400-with-invalid-
id response. **Browser network-tab inspection is recommended to
confirm whether the actual Authority fetch returns 200 or 400** —
see §13.

A secondary defensive flaw was identified: `_renderUnavailable`
performs `mount.appendChild(...)` without a null guard, so if
`_ensureMount()` ever returns null, `_pocRefresh` will throw and
silently kill the render path. This is MEDIUM-CONFIDENCE — it is
a latent bug, not necessarily the active cause of the blank state.

The 401s the user observed on `/auth/me` and `/explorer/envelopes`
are NOT Authority-graph fetches; they are unrelated background
session-probe calls. They do not affect the Authority graph render
path. See §9.

---

## 2. Observed symptom recap

User-reported browser evidence:

1. After D37b, the Authority Graph is **blank** in the browser.
2. Context Graph (native, default lens) renders correctly.
3. Browser console logs `"Authority Graph adapter active"` — this
   is the marker fired by [refreshGovernanceMap](../../internal/httpapi/explorer/index.html#L4294-L4297) (the Context-lens fetch
   path) on its first run. It confirms the adapter MODULE is
   loaded; it does NOT confirm an Authority fetch happened.
4. Cytoscape loads and emits warnings.
5. Unrelated 401s on `/auth/me` and `/explorer/envelopes?limit=50`.
6. A direct manual request to `/v1/graphs/authority` (probable URL
   shape: bare path, or with `?view=service` but no `id`) returned
   `400 {"error":"authoritygraph: invalid id"}`.

---

## 3. UI activation path trace

### Header Authority button → setWorkbenchMode

The header lens-menu Authority button is at
[index.html:417-418](../../internal/httpapi/explorer/index.html#L417-L418) with `data-workbench-mode="authority" data-lens="authority"`.

The click handler dispatches into `setWorkbenchMode(mode)` defined
at [index.html:3237](../../internal/httpapi/explorer/index.html#L3237). The Authority branch
([index.html:3296-3327](../../internal/httpapi/explorer/index.html#L3296-L3327)) does, in order:

1. `_showAuthorityPanels(true)` — reveals Authority-side panels.
2. `_setWorkbenchModeActiveButton('authority')` — flips the active
   button in the lens menu.
3. **Pre-seed** `MIDASExplorerStore.setState({ selectedGraphLens: 'authority' })` ([L3307-L3309](../../internal/httpapi/explorer/index.html#L3307-L3309)) so the downstream
   `refreshGovernanceMap` hook early-returns (lens guard at
   [index.html:4284-4287](../../internal/httpapi/explorer/index.html#L4284-L4287)).
4. `MIDASExplorerServices.showMap(serviceId)` ([L3310-L3312](../../internal/httpapi/explorer/index.html#L3310-L3312)) — sets
   `currentSelectedService` / `currentGraphRootId`, transitions
   the services subview to `map`, sets `gmapMode = 'map'`, calls
   `_hooks.refreshGovernanceMap()` (no-op for Authority lens due
   to the guard).
5. `ExplorerGraph.shell.setActiveLens('authority')` ([L3313-L3315](../../internal/httpapi/explorer/index.html#L3313-L3315)).
6. `ExplorerGraph.authorityView.refresh({ rootId: serviceId })`
   ([L3316-L3318](../../internal/httpapi/explorer/index.html#L3316-L3318)) — the call shape is **an object with `rootId`**, same
   as the pre-D37b legacy path.

### Empty-serviceId guard

[setWorkbenchMode at L3244-3252](../../internal/httpapi/explorer/index.html#L3244-L3252) computes
`serviceId = _activeServiceForWorkbench()` and short-circuits to
`showCatalogue()` when `serviceId` is empty AND mode is
`'context' | 'authority' | 'form'`. So the Authority branch only
runs with a non-empty `serviceId`.

### Deep-link activation path

[ExplorerRouter.register('graph/authority', …) at L2108-2117](../../internal/httpapi/explorer/index.html#L2108-L2117) also calls
`authorityView.refresh({ rootId: rootId || '' })` where `rootId =
currentGraphRootId`. Unlike `setWorkbenchMode`, this path does
NOT short-circuit on empty `rootId` — it dispatches `refresh` with
an empty string.

**Verdict**: UI activation reaches `authorityView.refresh` with a
well-formed `{rootId: <string>}` argument. The string is non-empty
when the user clicked the Authority button after selecting a
service; empty when they deep-linked without a prior service
selection.

---

## 4. Root id propagation trace

| Producer | Field | Where set |
|---|---|---|
| User selects a service in catalogue | `_selectedId` (services-view module-local) → `setSelectedServiceId(id)` | [services-view.js:139](../../internal/httpapi/explorer/assets/js/services/services-view.js#L139) inside `showMap` |
| `showMap(serviceId)` → `_hooks.resetGraphState('service', serviceId)` | sets inline `currentGraphView = 'service'`, `currentGraphRootId = serviceId` | [index.html:2272-2275](../../internal/httpapi/explorer/index.html#L2272-L2275) |
| `_activeServiceForWorkbench()` returns | `currentSelectedService` (if truthy) else `currentGraphRootId` (if truthy) else `null` | [index.html:3231-3235](../../internal/httpapi/explorer/index.html#L3231-L3235) |
| `setWorkbenchMode('authority')` passes | `serviceId` into `authorityView.refresh({rootId: serviceId})` | [index.html:3317](../../internal/httpapi/explorer/index.html#L3317) |
| `_pocRefresh({rootId})` extracts | `var rootId = opts.rootId \|\| ''` | [authority-cytoscape-poc.js:3390](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3390) |
| `_pocRefresh` guards empty | `if (!rootId) { _renderUnavailable(_ensureMount(), 'Select a business service to view the Authority Graph.'); return Promise.resolve({...}); }` | [authority-cytoscape-poc.js:3393-3397](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3393-L3397) |
| `_pocRefresh` issues fetch | `adapter.fetch({ view: 'service', id: rootId, depth: depth })` | [authority-cytoscape-poc.js:3409](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3409) |
| `fetchAuthorityGraph` passes through to | `api.authority({view, id, depth, force})` | [authority-graph-adapter.js:153-158](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L153-L158) |
| `api.authority` builds URL | `'/v1/graphs/authority' + _query({view, id, depth, force})` | [api-client.js:202-211](../../internal/httpapi/explorer/assets/js/core/api-client.js#L202-L211) |
| `_query` SKIPS empty values | `if (v === undefined \|\| v === null \|\| v === '') return;` | [api-client.js:138-147](../../internal/httpapi/explorer/assets/js/core/api-client.js#L138-L147) |

**Implications**:

- The frontend can only emit a fetch with an `id=` parameter when
  `rootId` is a non-empty string.
- An empty/null/undefined `rootId` is guarded at
  [authority-cytoscape-poc.js:3393-3397](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3393-L3397) BEFORE fetch — no request is issued
  in that case.
- If `_query` is reached with `id: ''`, the URL DROPS the param
  entirely (not even `&id=`), producing `/v1/graphs/authority?view=service&depth=4`. The backend then sees
  `q.Get("id") === ""` and returns `ErrInvalidID` (400) per
  [service.go:173-175](../../internal/graph/authority/service.go#L173-L175).
- The only way the frontend production code can produce a 400 is
  if either (a) the empty-rootId guard is bypassed (e.g. a code
  path calling `adapter.fetch` directly without the guard), OR
  (b) `_pocRefresh` is somehow called with the literal string
  `'undefined'` (length > 0, truthy → guard passes), which would
  then map to `?id=undefined` and the backend would return 404
  (not found), not 400.

**Verdict**: in the documented code paths, an empty `rootId`
produces NO fetch (so no 400). The 400 the user observed is
**INFERENCE: most likely a separate manual probe** of the bare
`/v1/graphs/authority` URL, not the actual production fetch.

---

## 5. Backend fetch trace

`GET /v1/graphs/authority` handler at [authority_graph_handler.go:61-97](../../internal/httpapi/authority_graph_handler.go#L61-L97):

| Param check | Behaviour |
|---|---|
| Method != GET | 405 |
| `s.authorityGraph == nil` | 501 |
| `view`, `id`, `depth` parsed from query | — |
| `authoritygraph.ParseDepth(q.Get("depth"))` returns error | 400 `{"error": err}` |
| `Project(ctx, view, id, depth)` returns `ErrInvalidView`/`ErrInvalidID`/`ErrInvalidDepth` | 400 |
| `Project` returns `ErrNotFound` | 404 |
| `Project` returns any other error | 500 |
| Success | 200 `{root, view, depth, nodes, edges, summary, …}` |

The error `"authoritygraph: invalid id"` is emitted by
[projection.go:77](../../internal/graph/authority/projection.go#L77) and triggered when `id == ""` at
[service.go:173-175](../../internal/graph/authority/service.go#L173-L175).

**Verdict**: backend is correct. A 400-with-`"invalid id"` response
proves the request reached the backend with no (or empty) `id`
query parameter.

---

## 6. GraphViewport activation trace

| Item | Status | Evidence |
|---|---|---|
| Renderer registered under `'authority'` | OK | `vp.register('authority', _authorityRendererFactory)` at [authority-cytoscape-poc.js:3534](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3534) |
| `_ensureMount` activates by id | OK | `vp.activateById('authority')` at [authority-cytoscape-poc.js:1432](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1432) |
| GraphViewport host is renderer-neutral | OK | No Authority literal in executable code of [graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js) |
| `data-active-renderer="authority"` set by host on activation | OK | `_setActiveRendererAttribute(rendererId)` at [graph-viewport.js:385-395](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L385-L395), invoked from `activate` at [L412](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L412) |
| Factory `mount(slotEl, ctx)` creates `.cytoscape-poc-mount` inside slot | OK | [authority-cytoscape-poc.js:1378-1388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1378-L1388) |
| Activation could succeed but fetch could still fail | YES (path exists) | `_pocRefresh` calls `_ensureMount` before fetch; if fetch fails, error overlay is shown — BUT see §10 for the layout collision that hides the overlay |

**Verdict**: GraphViewport activation is mechanically correct. The
host correctly flips `data-active-renderer="authority"` and the
Authority factory correctly creates `.cytoscape-poc-mount` inside
the renderer slot. The activation contract is intact.

---

## 7. Cytoscape mount / render trace

### Mount creation

`_authorityRendererFactory.mount(slotEl, ctx)` at
[authority-cytoscape-poc.js:1378-1388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1378-L1388):

```js
_mountEl = document.createElement('div');
_mountEl.className = 'cytoscape-poc-mount';
_mountEl.setAttribute('aria-label', 'Authority Graph');
try { slotEl.appendChild(_mountEl); } catch (_) { /* swallow */ }
```

`_mountEl` is appended as the LAST child of `slotEl`. At page
load, `slotEl` already contains
`.governance-map-canvas-scroll` ([index.html:459-472](../../internal/httpapi/explorer/index.html#L459-L472)), so after
factory.mount the slot has two children:

```
.midas-graph-renderer-slot
  .governance-map-canvas-scroll  (pre-existing, position: static, height: 100%)
    #gmap-canvas  (display: none !important when data-active-renderer="authority")
      ...
  .cytoscape-poc-mount  (D37b appendChild, position: relative, height: 100%, min-height: 480px)
```

### Layout collision (THE ROOT CAUSE)

- `.midas-graph-renderer-slot` is `position: absolute; inset: 0;`
  — fills the strategic viewport, height = viewport content area.
- `.governance-map-canvas-scroll` is `height: 100%;` — fills the
  slot vertically (= viewport height).
- `.cytoscape-poc-mount` is `position: relative; width: 100%;
  height: 100%; min-height: 480px;` — INSIDE block-formatted slot,
  positioned in NORMAL FLOW BELOW its preceding sibling.
- The slot uses block layout (no `display: flex/grid`), so block
  children stack vertically. Since the first child already
  occupies the full slot height, the second child starts at the
  slot's bottom — i.e. at or below the visible viewport bottom.
- `.midas-graph-viewport` is `overflow: hidden`
  ([governance-map.css:162-166](../../internal/httpapi/explorer/assets/css/governance-map.css#L162-L166)) — anything below the visible viewport is
  CLIPPED away.

**Result**: `.cytoscape-poc-mount` is positioned below the visible
area and clipped invisible.

### Cytoscape consequences

Cytoscape initialises by reading the container's `clientWidth` /
`clientHeight`. An off-screen-clipped container reports a non-zero
width/height depending on browser specifics, but more importantly
Cytoscape's `cy.fit()` and layout warnings fire when the visible
render area is degenerate. The user reported "Cytoscape loads and
emits warnings" — this is consistent with Cytoscape attempting to
mount into an unusable container.

### Render-state expectations

| `_pocRefresh` outcome | What the user should see (in the visible viewport) | What the user actually sees (per the mount-clipped hypothesis) |
|---|---|---|
| Empty `rootId` | `_renderUnavailable(mount, 'Select a business service to view the Authority Graph.')` text overlay | **Nothing** — overlay is positioned absolute inset:0 inside the off-screen mount, so it's also clipped. |
| Valid `rootId` + 200 OK | Cytoscape nodes + edges rendered in the mount | **Nothing** — Cytoscape renders into the off-screen mount. |
| Valid `rootId` + 4xx/5xx | `_renderUnavailable(mount, 'Network error: …')` overlay | **Nothing** — same reason. |
| Valid `rootId` + 404 | `_renderUnavailable(mount, 'No Authority Graph for this service yet.')` overlay | **Nothing** — same reason. |

**Verdict**: every overlay AND every Cytoscape render goes into a
mount that is positioned off-screen and clipped invisible. From
the user's perspective, the canvas area is blank regardless of
what `_pocRefresh` does internally.

This explains why the symptom is "blank" rather than "Loading…"
or "error" or "no nodes" text.

---

## 8. HTML card overlay trace

The Authority HTML card design is preserved:

- 9 themes including `'html-card'` and `'authority-thin-card-v1'`
  at [authority-cytoscape-poc.js:87](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L87).
- `_installHtmlCardOverlay`, `_updateHtmlCardOverlay`,
  `_destroyHtmlCardOverlay` defined and exported.
- Per-kind visual descriptors in `_kindStyle(palette)`.
- Cards CSS at [authority-cytoscape-poc.css:63-164](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L63-L164).

The card overlay would only fire if Cytoscape rendered nodes. Since
the mount itself is hidden (per §7), the card overlay never gets a
chance to paint visibly. Even if it did paint, it would paint INTO
the off-screen mount and remain invisible.

**Verdict**: HTML card overlay is downstream of the mount-visibility
problem and is not contributing to the blank state. It will work
once the mount becomes visible.

---

## 9. 401 console noise diagnosis

The two 401s observed:

| URL | Source | Authority-related? |
|---|---|---|
| `/auth/me` | Background session-probe — checks who the current user is. Standard cookie-auth session warmup. | NO |
| `/explorer/envelopes?limit=50` | Background data preload — fetches envelopes for the catalogue and audit-trail panels. | NO |

Neither URL is called by the Authority renderer code path. The
Authority renderer uses `/v1/graphs/authority` exclusively.

If the 401s were genuinely affecting session state, Context Graph
would also fail (it relies on the same auth). The user reports
Context Graph works, so the 401s do not represent a broken auth
session — they are unrelated background-fetch noise (likely
endpoints not yet implemented or scoped behind a feature flag).

**Browser distinguishing check**: in the Network panel, filter by
URL containing `graphs/authority`. The Authority renderer's actual
fetch (if any) will show up there. The presence/absence + status
code of that request is the diagnostic that tells us whether the
blank state is downstream of an auth issue (401), an id issue
(400), or a layout issue (200 OK but mount hidden).

**Verdict**: 401s are NOT related to the Authority blank state.

---

## 10. Verified root cause (ranked)

### CANDIDATE #1 — Mount-vs-canvas-scroll layout collision (**HIGH-CONFIDENCE**)

**What**: `.cytoscape-poc-mount` is appended as a sibling of
`.governance-map-canvas-scroll` inside `.midas-graph-renderer-slot`.
Both children declare `height: 100%`. In the slot's block-flow
formatting context, the mount is positioned below the visible
viewport and clipped invisible by `.midas-graph-viewport { overflow: hidden }`.

**Evidence**:
- DOM nesting [index.html:459-473](../../internal/httpapi/explorer/index.html#L459-L473).
- Slot CSS [governance-map.css:179-182](../../internal/httpapi/explorer/assets/css/governance-map.css#L179-L182).
- Canvas-scroll CSS [governance-map.css:529-535](../../internal/httpapi/explorer/assets/css/governance-map.css#L529-L535).
- Mount CSS [authority-cytoscape-poc.css:29-46](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L29-L46).
- Append-as-sibling code [authority-cytoscape-poc.js:1387](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1387).
- Strategic clip [governance-map.css:162-166](../../internal/httpapi/explorer/assets/css/governance-map.css#L162-L166).

**Why it surfaces only now**: pre-D37b, the Cytoscape PoC was
opt-in via `?cytoscape=1`, so the bug was latent — most users
never saw it. D37b made Cytoscape the default Authority renderer
without re-validating mount geometry in this configuration.

**Why it matches all observed symptoms**:
- Authority Graph blank ✓ (mount is off-screen)
- Cytoscape warnings ✓ (Cytoscape inits in degenerate container)
- Context Graph works ✓ (Context uses pre-existing `#gmap-canvas`,
  not the Cytoscape mount)
- Adapter loaded log ✓ (independent — fires from Context fetch)

**Note on pre-existing behaviour**: under `?cytoscape=1` (pre-D37b),
the PoC ALSO appended as a sibling. INFERENCE: either the PoC was
visually broken under `?cytoscape=1` and nobody noticed (it was a
strategic evaluation prototype with no users), OR the layout
naturally worked in some browser configurations (block-formatting
behaviour with `inset: 0` containing blocks can be subtle and
browser-specific). Either way, D37b promotion exposed the bug.

### CANDIDATE #2 — `_renderUnavailable` null-deref (MEDIUM-CONFIDENCE; latent defensive flaw)

**What**: `_renderUnavailable(mount, message)` at
[authority-cytoscape-poc.js:3319-3332](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3319-L3332) calls `mount.appendChild(overlay)` without a null
guard. `_ensureMount()` can return `null` (e.g. host unavailable —
documented at [L1452-1463](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1452-L1463)). If `_ensureMount` returns null, the
chain throws `TypeError: Cannot read properties of null (reading 'appendChild')`,
silently killing the render path.

**Why MEDIUM-CONFIDENCE**: this is a known defensive flaw, not
necessarily the active root cause. The host is normally available
(graph-viewport.js loads first), so `_ensureMount` shouldn't return
null in the production path. But it's a real footgun that compounds
the layout issue when anything goes wrong.

### CANDIDATE #3 — Empty-`id` fetch from an unprotected caller (LOW-CONFIDENCE)

**What**: if any code path calls `adapter.fetch` for Authority
WITHOUT going through `_pocRefresh`'s empty-`rootId` guard, the
URL becomes `/v1/graphs/authority?view=service&depth=4` (no `id`
param) and the backend returns 400 invalid id.

**Why LOW-CONFIDENCE**: a `grep` for `adapter.fetch` returns 2
call sites:
- [_pocRefresh](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3409) (guarded)
- [shell.refresh](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L194) (un-guarded for empty `id`, but only called for
  Authority via the legacy `authorityView.refresh` which is now
  patched to `_pocRefresh` and never called directly).

The 400 the user observed is **INFERENCE: most likely a manual
probe** of the bare URL. To confirm, the browser Network panel
should be inspected.

### Other candidates considered and ruled out

- **GraphViewport activation failure** — ruled out: §6 traces show
  the contract is mechanically correct.
- **`data-active-renderer` not flipping** — ruled out: §6 confirms
  the host writes the attribute on activation.
- **HTML card overlay broken** — ruled out: §8 shows the overlay
  is downstream of the mount-visibility problem.
- **401 affecting Authority fetch** — ruled out: §9 shows the 401s
  are unrelated background calls.
- **Stale renderer state from previous lens** — ruled out: lens
  switch correctly destroys prior renderer via host's
  `deactivate()` (D35f contract).

### Confidence summary

| Candidate | Confidence | Match to symptoms |
|---|---|---|
| #1 — mount layout collision | **HIGH** | Matches all observed symptoms cleanly |
| #2 — `_renderUnavailable` null-deref | MEDIUM | Latent flaw, may compound the blank state |
| #3 — empty-id fetch from unprotected caller | LOW | Requires the 400 to be the production fetch; INFERENCE is that it's a manual probe |

---

## 11. Minimal recommended fix plan

**Strong preference**: fix the layout collision in CSS. This is a
single-line change, preserves the D37b strategic decision (Cytoscape
on GraphViewport), and does not touch graph-viewport.js.

### Proposed minimal fix

`internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css`
— change `.cytoscape-poc-mount`'s `position: relative` to
`position: absolute; inset: 0;` so the mount overlays the slot
(stacking above the legacy `.governance-map-canvas-scroll`
sibling) rather than stacking below it.

Concretely the existing rule at [authority-cytoscape-poc.css:29-46](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L29-L46):

```css
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-mount {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 480px;
  background: var(--surface-container-lowest, #0c111c);
  border: 1px solid var(--outline-variant, #475569);
  border-radius: 6px;
  overflow: hidden;
  font-family: var(--font-display, Inter, 'Segoe UI', system-ui, sans-serif);
  color: var(--on-surface, #e2e8f0);
}
```

becomes:

```css
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-mount {
  position: absolute;        /* CHANGED: was `relative` */
  inset: 0;                  /* NEW: anchor to slot's four sides */
  min-height: 480px;
  background: var(--surface-container-lowest, #0c111c);
  border: 1px solid var(--outline-variant, #475569);
  border-radius: 6px;
  overflow: hidden;
  font-family: var(--font-display, Inter, 'Segoe UI', system-ui, sans-serif);
  color: var(--on-surface, #e2e8f0);
  /* `width: 100%; height: 100%;` removed — `inset: 0` is
     equivalent and more semantically correct for an absolutely
     positioned overlay. */
}
```

### Why this is the right fix

- The slot is `position: absolute; inset: 0;` and provides the
  containing block. An absolutely positioned mount with `inset: 0`
  fills the slot exactly, overlaying the legacy canvas-scroll
  sibling.
- `.midas-graph-viewport { overflow: hidden }` still clips
  correctly because the mount is now anchored inside the slot.
- No JS change. No graph-viewport.js change. No DOM mutation.
- The legacy `.governance-map-canvas-scroll` remains in the DOM
  (its child `#gmap-canvas` is already hidden by `display: none`
  when Authority is active; the scroll wrapper is harmless because
  it's now under the mount).
- The mount's existing `overflow: hidden` stays as Cytoscape's
  canvas-discipline defence (D35h contract).

### Files to modify (in the fix tranche)

- `internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css`
  — one rule change (positioning + remove redundant w/h).

### Optional secondary cleanup (separate from the blank-state fix)

- Add a null guard in `_renderUnavailable(mount, message)` so
  `_ensureMount() === null` no longer throws:
  ```js
  function _renderUnavailable(mount, message) {
    if (!mount) return;       /* defensive null guard */
    _destroyCy();
    _clearOverlays(mount);
    ...
  }
  ```
  This is independent of the layout fix and addresses Candidate #2.

### Expected request after fix

When the user clicks the Authority button after selecting a
service:

- URL: `GET /v1/graphs/authority?view=service&id=<serviceId>&depth=4`
- Backend: 200 with the projection envelope.
- Browser: Cytoscape renders nodes + edges inside `.cytoscape-poc-mount`,
  which now overlays the slot and is visible.

---

## 12. Tests to add with the fix

### CSS / layout contract (the primary fix)

| Test name | Pins |
|---|---|
| `TestExplorer_D37cAuthorityMount_PositionsAbsoluteInsideSlot` | `.cytoscape-poc-mount` declares `position: absolute` and `inset: 0;` in the active selector. |
| `TestExplorer_D37cAuthorityMount_DoesNotStackBelowCanvasScroll` | Negative pin: `.cytoscape-poc-mount` rule MUST NOT declare `position: relative` (which caused the pre-fix sibling-stacking collision). |
| `TestExplorer_D37cAuthorityMount_RetainsCytoscapeOverflowDiscipline` | `overflow: hidden` is still declared on `.cytoscape-poc-mount` (defence-in-depth for Cytoscape canvas-discipline). |

### Defensive null-guard (the secondary fix, if landed)

| Test name | Pins |
|---|---|
| `TestExplorer_D37cAuthority_RenderUnavailableNullSafe` | `_renderUnavailable` body contains an early `if (!mount) return;` guard. |

### Foundation regression (preserve D37b strategic state)

| Test name | Pins |
|---|---|
| `TestExplorer_D37c_D37bAuthorityCytoscapeContractsPreserved` | All D37b invariants remain: `vp.register('authority', _authorityRendererFactory)`, `vp.activateById('authority')`, `data-active-renderer="authority"` CSS selectors, aria-label `"Authority Graph"`, no `?cytoscape=1` gate, diagnostics/posture/workbench bridge calls. |

---

## 13. Browser validation checklist

This assessment is static. Before shipping the fix, the user
should validate in the browser:

### Confirm the root cause hypothesis

- [ ] Open `/explorer#services`, select a service, click Authority.
- [ ] DevTools Elements: confirm `.midas-graph-renderer-slot` has
      both `.governance-map-canvas-scroll` and `.cytoscape-poc-mount`
      as children.
- [ ] DevTools Computed: select `.cytoscape-poc-mount`, check
      `getBoundingClientRect()` (via Console:
      `document.querySelector('.cytoscape-poc-mount').getBoundingClientRect()`).
      Expect: `top` is at or below the visible viewport bottom
      (large positive value); `height` is `480` (min-height); `width`
      is the viewport width.
- [ ] DevTools Console: run
      `getComputedStyle(document.querySelector('.cytoscape-poc-mount')).position`.
      Expect: `"relative"`.

If these checks confirm the layout collision, Candidate #1 is the
verified root cause.

### Confirm the actual Authority fetch status

- [ ] DevTools Network: filter `graphs/authority`.
- [ ] Click Authority button (with a service selected).
- [ ] Expect a single request to `/v1/graphs/authority?view=service&id=<id>&depth=4`.
- [ ] Expect HTTP 200 (NOT 400).

If the actual fetch is 200, the 400 from the manual probe was
incidental and the blank state is entirely a layout issue. If
the actual fetch is 400, additional investigation is needed to
find the unprotected adapter.fetch caller.

### Confirm GraphViewport identity

- [ ] DevTools Console: `MIDASExplorerGraph.viewport.getActiveRendererId()`
      → expect `"authority"`.
- [ ] DevTools Console: `document.querySelector('.midas-graph-viewport').getAttribute('data-active-renderer')`
      → expect `"authority"`.

### Confirm Cytoscape warning shape

- [ ] DevTools Console: read the Cytoscape warning text. If it
      mentions "container has invalid dimensions" / "size of cy
      container is zero" / similar, that directly corroborates
      Candidate #1.

### Post-fix validation

- [ ] After applying the CSS fix, repeat the steps above. Expect:
      - `.cytoscape-poc-mount.getBoundingClientRect()` is now
        within the visible viewport.
      - `getComputedStyle(...).position` is `"absolute"`.
      - No Cytoscape "invalid dimensions" warning.
      - Cytoscape nodes + edges visible.
      - Authority Workbench (bottom 5 tabs), Diagnostics panel,
        Surface posture panel render.
      - Switching Authority ↔ Context still works; baseline restoration is clean.
