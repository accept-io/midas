# D37e — Authority Cytoscape HTML-Card Overlay: Assessment and Implementation Plan

Tranche: D37e
Status: read-only assessment and implementation plan.
Mandate: produce a clear, evidence-based plan for adding Authority
HTML-card overlay rendering to the working D37d Authority Cytoscape
graph, reusing the proven D34i two-tier transform model from the
Context Cytoscape spike.

This report makes no source, test, or CSS changes. The single
output is this document.

---

## 1. Executive summary

**The Authority renderer already contains a near-complete HTML
card overlay** at [authority-cytoscape-poc.js:1738-1879](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1738-L1879)
(`_installHtmlCardOverlay` / `_buildHtmlCard` /
`_updateHtmlCardOverlay` / `_destroyHtmlCardOverlay`), with full
per-kind CSS at [authority-cytoscape-poc.css:80-164](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L80-L164).

The overlay is **gated by theme** — only active when
`_activeTheme === 'html-card'` ([authority-cytoscape-poc.js:1765](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1765),
[authority-cytoscape-poc.js:3227](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3227)). The default theme is `'classic'`
([authority-cytoscape-poc.js:99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L99)), so the overlay never fires in the production
default path. CSS rules are scoped under
`.cytoscape-poc-mount[data-cy-theme="html-card"]`
([authority-cytoscape-poc.css:80](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L80)) and the JS sets
`mount.dataset.cyTheme = _activeTheme` ([authority-cytoscape-poc.js:3201](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3201)),
so until the default theme changes, none of the HTML card CSS
applies.

**The existing Authority overlay uses the OLDER ONE-TIER
projection model** (`n.renderedPosition()` per card, no layer
transform). This predates the D34i two-tier model proven in the
Context spike ([context-cytoscape-overlay-spike.js:716-765](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L716-L765))
and pinned by tests at [explorer_d35e_test.go:288-318](../../internal/httpapi/explorer_d35e_test.go#L288-L318) and
[explorer_authority_cytoscape_poc_test.go:4039-4140](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L4039-L4140). The one-tier model
works but recomputes every card on every pan/zoom event — O(N) per
event. The D34i two-tier model collapses pan/zoom to O(1).

**The existing Authority overlay also carries the
disappearing-card bug** D35e fixed in Context: the
`.cytoscape-poc-html-overlay` CSS rule declares `overflow: hidden`
([authority-cytoscape-poc.css:83](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L83)). The current Authority overlay paints via
renderedPosition (already-projected coords), so the bug is dormant
under the one-tier model. **But if D37f migrates to the two-tier
model without fixing the CSS, the bug surfaces immediately.**

### Recommended approach

**D37f — adapt the Authority overlay to the D34i two-tier model
and make HTML cards the default (no theme switch).**

Three coordinated changes:

1. **Remove the theme-gate**: stop gating overlay install on
   `_activeTheme === 'html-card'`. Render HTML cards
   unconditionally as the default Authority visual.
2. **Migrate to D34i two-tier**: split the existing one-tier
   `_installHtmlCardOverlay` + `_updateHtmlCardOverlay` into a
   layer-tier sync (cy.pan + cy.zoom on the overlay) and a
   cards-tier sync (per-card model-coord position + selection).
   Adopt `LAYER_SYNC_EVENTS = 'pan zoom render resize'` and
   `CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'`.
3. **Fix the overlay clip**: drop `overflow: hidden` from
   `.cytoscape-poc-html-overlay`. Add `transform-origin: top left`.
   The `.midas-graph-viewport { overflow: hidden }` remains the
   strategic clip authority; `.cytoscape-poc-mount`'s own
   `overflow: hidden` remains as Cytoscape canvas discipline.

Internal naming (`cytoscape-poc-html-card*`, mount class
`cytoscape-poc-mount`, theme literal `'html-card'`) stays as
internal naming debt — strategic rename is deferred per D37b/D37c
convention.

Risk level: **LOW–MEDIUM**. The overlay code already exists and
is exercised by tests; the migration is largely reorganisation
plus one CSS line removal plus an event-binding split. The D34i
model is pinned in Context (D35e tests) and well-understood. The
biggest open question is whether to dim/hide native Cytoscape
nodes underneath the cards (see §7).

---

## 2. Current Authority Cytoscape render path

### Activation chain (post-D37b/D37d)

1. User clicks Authority button → `setWorkbenchMode('authority')` ([index.html:3296-3327](../../internal/httpapi/explorer/index.html#L3296-L3327)) → `authorityView.refresh({rootId: serviceId})` (patched to `_pocRefresh` at module init).
2. `_pocRefresh` calls `_ensureMount()` which activates Cytoscape via `vp.activateById('authority')` and creates `.cytoscape-poc-mount` inside `.midas-graph-renderer-slot` (D37d-fixed: now `position: absolute; inset: 0;`).
3. `adapter.fetch({view: 'service', id: rootId, depth: 4})` hits `/v1/graphs/authority`.
4. `_renderPayload(payload)` runs:
   - Caches `window.MIDASExplorerGraph._lastAuthorityProjection = payload`.
   - Maps projection to Cytoscape elements via `mapProjectionToElements(payload)`.
   - Computes preset positions via `_computePresetPositions(payload, elements)`.
   - Sets `mount.dataset.cyTheme = _activeTheme` ([authority-cytoscape-poc.js:3201](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3201)).
   - Initializes Cytoscape via `window.cytoscape({container: mount, elements, style: _buildStyleArray(_activeTheme), layout: { name: 'preset', positions, fit: true, padding: fitPadding }, …})` ([authority-cytoscape-poc.js:3203-3218](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3203-L3218)).
   - Calls `_wireInteractions()`.
   - **Conditionally** calls `_installHtmlCardOverlay(_cy, mount, elements, _activeTheme)` if `_activeTheme === 'html-card'` ([authority-cytoscape-poc.js:3227-3229](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3227-L3229)).
   - Triggers `_settleFit()` via rAF / setTimeout for fit-to-viewport.
   - Calls the diagnostics/posture/workbench panel bridge (D37b).

### What the user currently sees (default theme)

With `_activeTheme = 'classic'`, the Authority graph renders as
styled Cytoscape boxes (per-kind fill / stroke / shape from
`_kindStyle(palette)` at [authority-cytoscape-poc.js:158-167](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L158-L167)).
This is fine — but the user has indicated the rich PoC HTML card
design should be the production presentation.

### Where Cytoscape nodes are created

- `mapProjectionToElements(payload)` — builds the Cytoscape
  elements array from the projection's `nodes[]` and `edges[]`.
- `_computePresetPositions(payload, elements)` — assigns
  deterministic vertical-lane positions per node.
- `window.cytoscape({container, elements, …})` ([authority-cytoscape-poc.js:3203-3218](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3203-L3218)) registers nodes with the engine.

### Per-node data available to a card

The mapper produces Cytoscape node `data` objects that carry the
projection's typed data block. From the existing `_buildHtmlCard(d)` ([authority-cytoscape-poc.js:1799-1836](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1799-L1836)):

| Field | Source | Used in card today |
|---|---|---|
| `d.id` | Cytoscape node id (refKey-shaped) | `data-node-id` attribute |
| `d.kind` | projection node kind (`business_service`, …) | `data-kind` attribute + kind chip text |
| `d.label` | projection label / id | title text |
| `d.isRoot` | inferred from projection root | `data-root="true"` for special styling |
| `d.raw.business_service.status` (etc.) | projection typed-data status field | status row text (only emitted if present) |

So per-card data is sufficient for the existing card design;
nothing new is required from the projection.

### Existing card DOM shape

```html
<article class="cytoscape-poc-html-card"
         data-node-id="…" data-kind="…" data-root="true">
  <span class="cytoscape-poc-html-card-kind">Decision Surface</span>
  <div class="cytoscape-poc-html-card-title">Card label text</div>
  <div class="cytoscape-poc-html-card-status">active</div>  <!-- optional -->
</article>
```

The CSS provides per-kind border colours, dashed borders for
sidecar kinds (`fail_mode_policy`, `escalation_target`), root
emphasis, and typography ([authority-cytoscape-poc.css:87-164](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L87-L164)).

### Hover / focus / selection state today

- The `_wireInteractions()` (called from `_renderPayload`) binds
  Cytoscape's tap/hover handlers to update the Cytoscape internal
  `:selected` state and route to the inspector via
  `_renderInspectorCarriers`.
- The HTML overlay's `_updateHtmlCardOverlay` does NOT currently
  mirror selected/hover state onto card CSS classes (the Context
  spike's `_syncCards` does — see [context-cytoscape-overlay-spike.js:762-763](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L762-L763)). This is a known
  gap to close in D37f.

---

## 3. Existing Authority HTML-card design inventory

### JS implementation (already present, dormant by theme-gate)

| Function | Lines | Role |
|---|---|---|
| `_installHtmlCardOverlay(cy, mount, elements, themeName)` | [authority-cytoscape-poc.js:1763-1797](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1763-L1797) | Creates `.cytoscape-poc-html-overlay`, builds one card per node, binds rAF-coalesced sync handler to `'render pan zoom position'`. |
| `_buildHtmlCard(d)` | [authority-cytoscape-poc.js:1799-1836](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1799-L1836) | Builds the per-card DOM (kind chip + title + optional status row). |
| `_updateHtmlCardOverlay(cy)` | [authority-cytoscape-poc.js:1838-1864](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1838-L1864) | One-tier sync: per-card `transform: translate3d(p.x - w/2, p.y - h/2, 0)` using `renderedPosition()`. |
| `_destroyHtmlCardOverlay()` | [authority-cytoscape-poc.js:1866-1879](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1866-L1879) | Cancels rAF, unbinds Cytoscape listener, removes overlay DOM. |

Module state at [authority-cytoscape-poc.js:1758-1761](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1758-L1761):
- `_htmlOverlayEl` — overlay container.
- `_htmlCardsByKey` — `{refKey: <article>}` map.
- `_htmlSyncRaf` — rAF coalescing flag.
- `_htmlSyncBound` — bound sync handler.

### CSS implementation (already present, dormant by theme-attribute selector)

All HTML-card CSS lives in [authority-cytoscape-poc.css:80-164](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L80-L164),
scoped under
`.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-mount[data-cy-theme="html-card"]`.

| Selector | Role | Today |
|---|---|---|
| `…html-overlay` | overlay container | `position: absolute; inset: 0; overflow: hidden;` **← disappearing-card bug latent**; `pointer-events: none;`; `z-index: 5;` |
| `…html-card` | per-card root | `position: absolute; top: 0; left: 0; width: 240px; height: 96px; padding; background; border; border-radius; box-shadow; pointer-events: none;` |
| `…html-card-kind` | kind chip | small uppercase eyebrow |
| `…html-card-title` | primary text | 13px/600, `-webkit-line-clamp: 2`, ellipsis |
| `…html-card-status` | status row | small uppercase |
| `…html-card[data-kind="business_service"]` | per-kind border | `2px solid var(--primary)` |
| `…html-card[data-kind="decision_surface"]` | per-kind border | `var(--badge-info)` |
| `…html-card[data-kind="authority_profile"]` | per-kind border | `var(--badge-good)` |
| `…html-card[data-kind="authority_grant"]` | per-kind border | `var(--badge-warn)` |
| `…html-card[data-kind="agent"]` | per-kind border | `var(--slate-300)` |
| `…html-card[data-kind="fail_mode_policy"]` | per-kind border | `var(--slate-400)` dashed |
| `…html-card[data-kind="escalation_target"]` | per-kind border | `var(--slate-400)` dashed |
| `…html-card[data-root="true"]` | root emphasis | (line 160+) |

**Inventory verdict**: the card design is complete enough to be
production-ready. D37f should reuse it verbatim — no card-design
work needed.

### Theme system (relevant for D37f)

`_THEMES = ['classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card', 'object-tile-v3', 'authority-thin-card-v1']` ([authority-cytoscape-poc.js:98](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L98)).
`DEFAULT_THEME = 'classic'` ([authority-cytoscape-poc.js:99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L99)).
`_resolveTheme` reads `?cyTheme=` query param ([authority-cytoscape-poc.js:101-110](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L101-L110)).

The `'html-card'` theme's per-theme descriptor at [authority-cytoscape-poc.js:1925-1937](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1937)
intentionally renders the underlying Cytoscape node "small and
faint" (e.g. `rootOverlayOpacity: 0`) so the HTML overlay carries
the visible card. INFERENCE: this is the right behaviour for
D37f — the Cytoscape node should be functionally present (for
hit-testing, drag, layout) but visually subdued so the HTML card
is the visible layer.

---

## 4. Prior Context HTML-card overlay lessons

### D34i two-tier transform model (canonical implementation)

[context-cytoscape-overlay-spike.js:710-774](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L710-L774):

**`_syncLayer()`** — ONE write per pan/zoom/render/resize event:
```js
var t = 'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')';
_overlayEl.style.transformOrigin = 'top left';
_overlayEl.style.transform       = t;
```

**`_syncCards()`** — ONE write per card per position/bounds/layoutstop/add/select/unselect event:
```js
var p = n.position();   // MODEL coords (not renderedPosition)
var t = 'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)';
card.style.transform = t;
if (n.selected()) card.classList.add('selected');
else              card.classList.remove('selected');
```

**Why this works**:
- The layer's `scale(cy.zoom())` projects every card from model
  space to rendered space in ONE style write.
- Pan/zoom cost drops from O(N) per event (rewrite every card) to
  O(1) (one layer style write).
- Per-card transform uses MODEL coords (not rendered) → no
  redundant projection.
- `translate(-50%, -50%)` centres the card on the model position
  without depending on the card's measured width/height.
- `transform-origin: top left` MUST match Cytoscape's internal
  projection origin so `cy.pan(0,0)` lands at the same screen
  pixel.

### Event constants (pinned by tests)

```js
var LAYER_SYNC_EVENTS = 'pan zoom render resize';
var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect';
var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position';  // diagnostic surface
```

[context-cytoscape-overlay-spike.js:105-113](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L105-L113)

### Per-tier rAF coalescing

[context-cytoscape-overlay-spike.js:948-969](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L948-L969):

```js
var _syncLayerRaf = 0;
var _syncCardsRaf = 0;

_syncLayerBound = function () {
  if (_syncLayerRaf) return;
  _syncLayerRaf = requestAnimationFrame(function () { _syncLayerRaf = 0; _syncLayer(); });
};
_syncCardsBound = function () {
  if (_syncCardsRaf) return;
  _syncCardsRaf = requestAnimationFrame(function () { _syncCardsRaf = 0; _syncCards(); });
};

_cy.on(LAYER_SYNC_EVENTS, _syncLayerBound);
_cy.on(CARDS_SYNC_EVENTS, _syncCardsBound);
```

Each tier has its own rAF flag so they coalesce independently.

### Pointer model

CSS at [context-cytoscape-overlay-spike.css:145-200](../../internal/httpapi/explorer/assets/css/context-cytoscape-overlay-spike.css#L145-L200):

- Overlay: `pointer-events: none`, `z-index: 100` inside the
  mount's stacking context (mount has `isolation: isolate`).
- Cards: `pointer-events: none` (cytoscape owns all hit-testing).
- Cytoscape canvas sits directly beneath each card at the same
  screen origin (overlay is a child of cy mount).
- Tap routing: `cy.on('tap', 'node', …)` routes to the production
  right-drawer hook `selectNode(nodeId)`.
- Keyboard activation: card-level click handler ALSO routes to
  the same hook (focused card's Enter/Space fires click; mouse
  pointer never fires it because pointer-events:none).

### Clipping model (D35e fix)

[context-cytoscape-overlay-spike.css:113-164](../../internal/httpapi/explorer/assets/css/context-cytoscape-overlay-spike.css#L113-L164):

> "Pre-D35e, the overlay's own `overflow: hidden` clipped cards
> in the overlay's UNTRANSFORMED coordinate space (cy mount size,
> e.g. 800x600). Cards whose model-space x-coord exceeded that
> pre-transform extent were clipped BEFORE the overlay's
> `scale(cy.zoom)` projected them into rendered space — producing
> the user-reported disappearing-card symptom where cards
> vanished halfway across the visible canvas at zoom < 1."

The fix: REMOVE `overflow: hidden` from the overlay. Add
`transform-origin: top left` so the projection lands correctly.
The mount keeps its own `overflow: hidden` (Cytoscape canvas
discipline). The `.midas-graph-viewport` is the strategic clip.

### Lifecycle (D35e factory + teardown)

[context-cytoscape-overlay-spike.js:1078-1118](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L1078-L1118):

```js
function _teardownResources() {
  cancelAnimationFrame(_syncLayerRaf);  // both tiers
  cancelAnimationFrame(_syncCardsRaf);
  _cy.off(LAYER_SYNC_EVENTS, _syncLayerBound);
  _cy.off(CARDS_SYNC_EVENTS, _syncCardsBound);
  _resizeObs?.disconnect();
  window.removeEventListener('resize', _onWinResize);
  _cy.destroy();
  _mountEl.parentNode.removeChild(_mountEl);
  // null out all module state
}
```

### Test discipline (pinned invariants D37f must preserve)

| Invariant | Test |
|---|---|
| `_syncLayer` writes `translate(pan.x, pan.y) scale(zoom)` with `transform-origin: top left` | [explorer_d35e_test.go:288-318](../../internal/httpapi/explorer_d35e_test.go#L288-L318) `TestExplorer_D35eContext_PreservesD34iTwoTierTransform` |
| `_syncCards` uses model coords + `translate(-50%, -50%)` + mirrors `selected()` | same test |
| `_syncCards` MUST NOT use `renderedPosition` or `scale(` | [explorer_authority_cytoscape_poc_test.go:4103-4112](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L4103-L4112) `TestExplorer_D34iTwoTier_CardSyncUsesModelPosition` |
| Event constants `LAYER_SYNC_EVENTS` / `CARDS_SYNC_EVENTS` | [explorer_authority_cytoscape_poc_test.go:4117-4140](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L4117-L4140) `TestExplorer_D34iTwoTier_EventBindingsSplit` |
| `_syncLayer` MUST NOT walk `_cardsByKey` (O(1) per event) | [explorer_authority_cytoscape_poc_test.go:4142+](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L4142) `TestExplorer_D34iTwoTier_PanZoomDoesNotWalkCards` |
| Overlay rule has NO `overflow: hidden` AND has `transform-origin: top left` | [explorer_d35e_test.go:320-348](../../internal/httpapi/explorer_d35e_test.go#L320-L348) `TestExplorer_D35eContext_OverlayDoesNotOwnClipping` |

---

## 5. Reusable code / patterns from Context overlay work

The Context Cytoscape spike's overlay machinery is the canonical
reference. Patterns to lift into D37f:

| Pattern | Source | D37f reuse |
|---|---|---|
| LAYER_SYNC_EVENTS / CARDS_SYNC_EVENTS constants | [context-cytoscape-overlay-spike.js:105-106](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L105-L106) | Copy verbatim into Authority module |
| `_syncLayer()` body | [context-cytoscape-overlay-spike.js:716-729](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L716-L729) | Copy verbatim; swap `_overlayEl` → Authority's `_htmlOverlayEl` |
| `_syncCards()` body | [context-cytoscape-overlay-spike.js:742-765](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L742-L765) | Copy verbatim; swap `_cardsByKey` → Authority's `_htmlCardsByKey` |
| Per-tier rAF coalescing | [context-cytoscape-overlay-spike.js:948-967](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L948-L967) | Copy verbatim |
| `_cy.on/off(LAYER_SYNC_EVENTS, …)` + `_cy.on/off(CARDS_SYNC_EVENTS, …)` | [context-cytoscape-overlay-spike.js:968-969,1090-1093](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L968-L969) | Replace Authority's single `cy.on('render pan zoom position', …)` |
| Overlay CSS: drop `overflow: hidden`, add `transform-origin: top left` | [context-cytoscape-overlay-spike.css:145-164](../../internal/httpapi/explorer/assets/css/context-cytoscape-overlay-spike.css#L145-L164) | Apply same fix to `.cytoscape-poc-html-overlay` |
| `n.selected()` mirror onto `.selected` card class | [context-cytoscape-overlay-spike.js:762-763](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L762-L763) | New addition for Authority cards |
| `PROJECTION_MODEL = 'layer-pan-zoom-card-model-position'` diagnostic constant | [context-cytoscape-overlay-spike.js:113](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L113) | Mirror for Authority diagnostic surface |

**The Context spike already has factory-mount-level
`ctx.onResize(_onHostResize)` subscription** ([context-cytoscape-overlay-spike.js:266-278 area](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js#L266-L278), and the Authority renderer already
has its own `ctx.onResize(_refitWithSafeArea)` subscription at
[authority-cytoscape-poc.js:1374-1377](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1374-L1377). Resize is handled — no new
host-resize wiring needed.

### Where NOT to copy from

**`cytoscape-html-overlay.js`** ([authority/cytoscape-html-overlay.js](../../internal/httpapi/explorer/assets/js/graph/authority/cytoscape-html-overlay.js)) is the D34a generic overlay
spike. It uses ONE-TIER `renderedPosition()` projection ([cytoscape-html-overlay.js:289-300](../../internal/httpapi/explorer/assets/js/graph/authority/cytoscape-html-overlay.js#L289-L300)),
which is the older model superseded by D34i. It also dims native
Cytoscape nodes via `cy.nodes().style({opacity: 0})` ([cytoscape-html-overlay.js:307-316](../../internal/httpapi/explorer/assets/js/graph/authority/cytoscape-html-overlay.js#L307-L316)).
It is **gated by `?cytoscape=1&htmlCards=1`** ([cytoscape-html-overlay.js:90-97](../../internal/httpapi/explorer/assets/js/graph/authority/cytoscape-html-overlay.js#L90-L97)), so it doesn't run by
default. D37f should NOT reuse this helper — adopt the D34i model
from the Context spike instead. Recommend leaving
`cytoscape-html-overlay.js` in place as historical debt (it is
opt-in and harmless when gated off); a cleanup tranche can remove
it later.

---

## 6. Authority vs Context overlay differences

| Concern | Context spike | Authority (current) | D37f Authority (proposed) |
|---|---|---|---|
| Card source | CLONES rendered `.gmap-node` cards from native Context Graph | Synthesises minimal card DOM (kind chip + title + status) | Keep current synthesis (already complete + tokens-aligned) |
| Node kinds | Context kinds (capability, process, ai_system, etc.) | Authority kinds (business_service, decision_surface, authority_profile, authority_grant, agent, fail_mode_policy, escalation_target) | Same as today |
| Per-card data | Full Context node data | Authority typed-data per kind (already in `d.raw`) | Same as today |
| Projection extras consumed by card | None | `summary.GrantsWithStopCapability`, per-node `diagnostic_kinds` — NOT currently rendered into cards | Out of scope for D37f; pin as known gap |
| Cards per node | One per cy node | One per cy node | Same |
| Cytoscape node visibility under card | (Context-spike CSS hides cy nodes) | `'html-card'` theme renders cy nodes "small and faint" via descriptor at [authority-cytoscape-poc.js:1925-1937](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1937) | Same: cy nodes stay hit-testable but visually subdued |
| Layout | Topology-driven (Cytoscape force-style with native cards) | Preset positions (deterministic vertical lanes) — `_computePresetPositions` | Same — no layout change |
| Edges | Cytoscape native | Cytoscape native | Same |
| Sync model | D34i two-tier | One-tier (renderedPosition) | **MIGRATE to D34i two-tier** |
| Overlay clipping | `overflow: visible` (D35e fix) | `overflow: hidden` (bug latent) | **REMOVE `overflow: hidden`** |
| `transform-origin` on overlay | `top left` (D34i requirement) | not set | **ADD `transform-origin: top left`** |
| Selection mirror onto card class | YES (`.selected`) | NO (gap) | **ADD `.selected` mirror in `_syncCards`** |
| Hover mirror onto card class | optional | NO | Defer to D37g — not a blank-state requirement |
| Drawer integration | `cy.on('tap', 'node', …)` → `selectNode(nodeId)` | Existing `_renderInspectorCarriers` carrier-DOM bridge | Keep current; no change |
| Workbench / diagnostics / posture rendering | N/A | D37b bridge calls `authorityWorkbench.render()`, `authorityDiagnosticsPanel.render(payload)`, `authoritySurfacePosturePanel.render(payload)` | Keep (already in `_renderPayload` after settle-fit) |

**The big delta**: migrate the Authority overlay's sync from
one-tier to D34i two-tier, fix the overlay CSS clip, mirror
selected state. Everything else (card DOM, per-kind border CSS,
card data wiring, drawer routing, diagnostics/posture/workbench)
is already correct.

---

## 7. Proposed Authority overlay architecture

### Overlay container

| Property | Value | Rationale |
|---|---|---|
| Element | `<div>` | Layout-free container |
| Class | `cytoscape-poc-html-overlay` (KEEP — naming debt) | Existing CSS uses this class |
| Parent | `mount` (= `.cytoscape-poc-mount` inside `.midas-graph-renderer-slot`) | Same as today; matches Context spike pattern (overlay is child of cy mount) |
| `position` | `absolute` (existing) | Anchored to mount |
| `inset` | `0` (existing) | Fills mount |
| `overflow` | **REMOVE `hidden`** | D35e fix — prevents disappearing-card bug |
| `pointer-events` | `none` (existing) | Cytoscape owns hit-testing |
| `z-index` | `5` (existing) — or `100` to match Context spike | Inside mount stacking context |
| `transform-origin` | **ADD `top left`** | D34i — projection origin must match cy's |
| ARIA | `role="presentation"` (existing) | Overlay is decorative |
| `isolation` (on mount) | confirm `isolate` set on `.cytoscape-poc-mount` | Establishes stacking context — already set on Context spike's mount |

### Per-card element

| Property | Value | Rationale |
|---|---|---|
| Element | `<article>` (existing) | Semantic card |
| Class | `cytoscape-poc-html-card` (KEEP) | Existing CSS uses it |
| `data-node-id` | cy node id | Maps card ↔ cy node |
| `data-kind` | `node.kind` | Per-kind CSS border colour |
| `data-root` | `"true"` if root | Root emphasis |
| `position` | `absolute` (existing) | Absolutely positioned inside overlay |
| `top` / `left` | `0` | Anchor for transform |
| Size | `width: 240px; height: 96px` (existing) | Fixed card footprint matches cy node footprint for `cy.fit()` |
| `transform` | **CHANGE to `translate(model.x, model.y) translate(-50%, -50%)`** | D34i two-tier card-tier write |
| `pointer-events` | `none` (existing) | Cytoscape owns hit-testing |
| `.selected` class | mirror `n.selected()` in `_syncCards` | Selected state |
| Inner DOM | kind chip + title + optional status row (existing) | No change |

### Card id ↔ DOM mapping

Existing `_htmlCardsByKey = { [cyNodeId]: cardElement }` is fine.
Build at install time; look up by `cy.$id(id)` in `_syncCards`.

### Card state mirrors Cytoscape state

- `_syncCards` reads `n.position()` (MODEL coords).
- `_syncCards` reads `n.selected()` and toggles `card.classList.add/remove('selected')`.
- Hover mirror (optional, defer to D37g): `cy.on('mouseover/mouseout', 'node', …)` → toggle `.hover` class.

### Pan/zoom transform

LAYER tier — one transform on `_htmlOverlayEl`:
```js
var t = 'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')';
_htmlOverlayEl.style.transformOrigin = 'top left';
_htmlOverlayEl.style.transform       = t;
```

### Per-card transform

CARDS tier — one transform per card:
```js
var p = n.position();   // MODEL coords
var t = 'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)';
card.style.transform = t;
```

### Safe area / clipping

- `.midas-graph-viewport { overflow: hidden }` remains strategic
  clip authority. **No change.**
- `.cytoscape-poc-mount { overflow: hidden }` remains Cytoscape
  canvas discipline. **No change.**
- `.cytoscape-poc-html-overlay` **drops `overflow: hidden`** so
  cards anchored at model coordinates outside the mount's
  untransformed extent are not clipped before the layer's
  `scale(cy.zoom)` projects them into view.

### Destroy

`_destroyHtmlCardOverlay` body becomes:
```js
function _destroyHtmlCardOverlay() {
  cancelAnimationFrame(_syncLayerRaf);
  cancelAnimationFrame(_syncCardsRaf);
  _syncLayerRaf = 0;
  _syncCardsRaf = 0;
  if (_cy && _syncLayerBound) _cy.off(LAYER_SYNC_EVENTS, _syncLayerBound);
  if (_cy && _syncCardsBound) _cy.off(CARDS_SYNC_EVENTS, _syncCardsBound);
  _syncLayerBound = null;
  _syncCardsBound = null;
  if (_htmlOverlayEl && _htmlOverlayEl.parentNode) {
    _htmlOverlayEl.parentNode.removeChild(_htmlOverlayEl);
  }
  _htmlOverlayEl  = null;
  _htmlCardsByKey = {};
}
```

---

## 8. Event synchronisation plan

| Tier | Events bound | Triggers per-event | Cost per event | Coalesced via |
|---|---|---|---|---|
| LAYER | `pan zoom render resize` | one style write on `_htmlOverlayEl` | O(1) | `_syncLayerRaf` flag |
| CARDS | `position bounds layoutstop add select unselect` | one style write per card + `.selected` toggle | O(N) | `_syncCardsRaf` flag |

Each tier's bound handler:

```js
_syncLayerBound = function () {
  if (_syncLayerRaf) return;
  _syncLayerRaf = requestAnimationFrame(function () {
    _syncLayerRaf = 0;
    _syncLayer();
  });
};

_syncCardsBound = function () {
  if (_syncCardsRaf) return;
  _syncCardsRaf = requestAnimationFrame(function () {
    _syncCardsRaf = 0;
    _syncCards();
  });
};

_cy.on(LAYER_SYNC_EVENTS, _syncLayerBound);
_cy.on(CARDS_SYNC_EVENTS, _syncCardsBound);
```

**Pinned by `TestExplorer_D34iTwoTier_EventBindingsSplit`** and
`TestExplorer_D34iTwoTier_PanZoomDoesNotWalkCards`. Authority's
`_syncLayer` MUST NOT iterate cards to keep pan/zoom O(1) per
event regardless of node count.

### Initial paint

After install, run one full sync (both tiers) inside the next rAF
so cy has time to settle:
```js
requestAnimationFrame(function () { _syncLayer(); _syncCards(); });
```

---

## 9. Pointer and interaction plan

Unchanged from D34h/D34i (Cytoscape-native):

- Overlay has `pointer-events: none`.
- Cards have `pointer-events: none`.
- All graph interaction (tap, drag, pan, zoom, box select) is
  Cytoscape-owned via existing `_wireInteractions()` at
  [authority-cytoscape-poc.js:_wireInteractions area](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js).
- Node selection routes to the existing inspector carrier-DOM
  bridge (`_renderInspectorCarriers`) — unchanged.
- Hover state mirror: defer to D37g (optional polish).
- Drag: Cytoscape native; `CARDS_SYNC_EVENTS` includes `position`
  which fires per-frame during a node drag, so cards follow the
  dragged node correctly via the two-tier mechanism.

No DOM-side keyboard activation in D37f. The existing card
elements are non-focusable (no `tabindex`); if focus is needed
later, add it as a separate accessibility tranche.

---

## 10. Clipping and viewport plan

| Layer | `overflow` | Role | Change in D37f |
|---|---|---|---|
| `.midas-graph-viewport` | `hidden` (D35f) | Strategic clip authority | **NO CHANGE** |
| `.midas-graph-renderer-slot` | (default — visible) | Renderer container | **NO CHANGE** |
| `.cytoscape-poc-mount` | `hidden` (existing) | Cytoscape canvas discipline | **NO CHANGE** |
| `.cytoscape-poc-html-overlay` | `hidden` (BUG — latent) | Projection layer; MUST NOT clip | **REMOVE `overflow: hidden`** |
| `.cytoscape-poc-html-card` | `hidden` (existing on card body for inner text) | Per-card content discipline | **NO CHANGE** |

This matches the Context spike's clip model verbatim.

---

## 11. Lifecycle and teardown plan

### Install

`_installHtmlCardOverlay(cy, mount, elements)` runs from
`_renderPayload` after `_wireInteractions()`. Signature simplifies
(drop `themeName` param — always installs):

```js
function _installHtmlCardOverlay(cy, mount, elements) {
  if (!cy || !mount) return;
  _destroyHtmlCardOverlay();

  _htmlOverlayEl = document.createElement('div');
  _htmlOverlayEl.className = 'cytoscape-poc-html-overlay';
  _htmlOverlayEl.setAttribute('role', 'presentation');
  mount.appendChild(_htmlOverlayEl);

  _htmlCardsByKey = {};
  (elements && elements.nodes || []).forEach(function (entry) {
    if (!entry?.data?.id) return;
    var card = _buildHtmlCard(entry.data);
    _htmlOverlayEl.appendChild(card);
    _htmlCardsByKey[entry.data.id] = card;
  });

  _syncLayerBound = …;   // see §8
  _syncCardsBound = …;
  cy.on(LAYER_SYNC_EVENTS, _syncLayerBound);
  cy.on(CARDS_SYNC_EVENTS, _syncCardsBound);

  requestAnimationFrame(function () { _syncLayer(); _syncCards(); });
}
```

Caller in `_renderPayload` becomes unconditional:

```js
// D37f — HTML card overlay is the production Authority visual.
_installHtmlCardOverlay(_cy, mount, elements);
```

(Theme-gate `if (_activeTheme === 'html-card')` removed.)

### Destroy

`_destroyHtmlCardOverlay` already wired into `_destroyCy()` ([authority-cytoscape-poc.js:1634](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1634))
and `_uninstallPoc()` ([authority-cytoscape-poc.js:1669](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1669)). Body updated per §7.

### Service refresh / lens unmount

Existing `_destroyCy()` chain handles teardown before re-init.
Existing factory destroy (host-routed via
`viewport.deactivate()`) handles full lens unmount. Both already
call `_destroyHtmlCardOverlay`. No new wiring needed.

### No stale overlay after deactivation

Pinned by D37f tests: after `_uninstallPoc()`, the overlay element
is removed from the DOM and `_htmlOverlayEl === null`.

---

## 12. CSS / class strategy

### Reuse existing classes (naming debt accepted)

Per D37b convention (internal naming debt acknowledged, not
renamed in narrow tranches), KEEP:
- `.cytoscape-poc-html-overlay`
- `.cytoscape-poc-html-card`
- `.cytoscape-poc-html-card-kind`
- `.cytoscape-poc-html-card-title`
- `.cytoscape-poc-html-card-status`
- Per-kind `[data-kind="…"]` and `[data-root="true"]` selectors

### Selector change: drop `[data-cy-theme="html-card"]` requirement

All HTML-card CSS today is scoped under
`.cytoscape-poc-mount[data-cy-theme="html-card"]`. D37f makes
HTML cards the default — the theme-attribute scope becomes
unnecessary. Two options:

**Option A** (preferred): drop the `[data-cy-theme="html-card"]`
selector from the HTML-card rules so they apply whenever
`data-active-renderer="authority"` is set:

```css
/* Before */
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-mount[data-cy-theme="html-card"] .cytoscape-poc-html-overlay { … }

/* After */
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-overlay { … }
```

Pros: cards always render under Authority. Theme query param
(`?cyTheme=other-theme`) still works for theme exploration without
HTML cards interfering (HTML overlay only installs when the JS
runs the install path).

**Option B**: keep the `[data-cy-theme="…"]` scope but set
`mount.dataset.cyTheme = 'html-card'` whenever HTML overlay is
active. More implicit dependency between JS and CSS; not
recommended.

**Recommend Option A.**

### CSS edits required (D37f)

1. `.cytoscape-poc-html-overlay` — remove `overflow: hidden`; add `transform-origin: top left`.
2. `.cytoscape-poc-html-card` — drop `position: absolute; top: 0; left: 0;` if redundant (overlay anchors via transform-origin + per-card transform). VERIFY in browser before removing; safer to keep.
3. Drop `[data-cy-theme="html-card"]` from all rule selectors so cards render under default Authority activation.

### CSS edits to AVOID

- Do NOT rename `.cytoscape-poc-html-card` / overlay classes
  (naming debt cleanup belongs to a separate tranche).
- Do NOT touch the per-kind border colours.
- Do NOT touch the typography rules.

---

## 13. Accessibility plan

### Current state

- Cytoscape canvas is the interactive layer; native cy keyboard
  support is minimal (no built-in focus traversal).
- The existing right-drawer Inspector tab provides accessible
  per-node content.
- The HTML overlay/cards have `pointer-events: none` and no
  `tabindex` — they are not in the focus order.

### D37f stance

- Cards SHOULD have `aria-hidden="true"` because they are
  presentational and Cytoscape owns hit-testing. The screen-reader
  representation of a node lives in the right drawer when
  selected.
- Cards SHOULD NOT have `tabindex` (would create a parallel focus
  order that doesn't drive any behaviour).
- Overlay element keeps `role="presentation"` (already set).
- INFERENCE: keyboard accessibility for graph navigation is a
  larger UX question (Context spike's `_wireCardKeyboardActivation`
  was its own design decision). D37f should NOT replicate that;
  defer keyboard navigation to a separate accessibility tranche.

---

## 14. Performance plan

### Current Authority projection scale

Projection depth default is 4 (with one BS, ~5–20 surfaces, 1–3
profiles each, 1–N grants, agents). Typical node count: 10–100.
Test fixtures pin a similar order ([service_seeded_test.go](../../internal/graph/authority/service_seeded_test.go)).

### Cost analysis

| Operation | One-tier (today) | Two-tier (D37f) |
|---|---|---|
| Pan/zoom event | O(N) per-card transform write | O(1) layer transform write |
| Node drag (single) | O(N) on every `position` event | O(N) on every `position` event (cards must follow). Layer transform unaffected. |
| Selection toggle | O(N) per-card scan today (since selection is currently NOT mirrored) | O(N) on `select`/`unselect` (toggle one card class — cheap) |
| Initial install | O(N) build + O(N) first sync | O(N) build + O(N) first sync |
| Resize | depends — currently bound | O(1) layer write (resize doesn't move nodes) |

For 10–100 nodes, both models are fast in practice. The two-tier
model wins on continuous pan/zoom (the user's primary interaction).

### Render-all-cards strategy

D37f renders one card per cy node. Visible-card-only optimisation
(e.g. only build cards for nodes inside cy's viewport bounds) is
NOT recommended for D37f: it adds complexity, complicates `add`/
`remove` events, and the projection size doesn't justify it.

### requestAnimationFrame batching

D37f uses per-tier rAF flags (matches Context spike). A burst of
pan/zoom collapses to one paint per frame; a burst of drag
position events collapses to one card-tier write per frame.

---

## 15. Test plan

### Pins required for D37f

| Test name | Pins |
|---|---|
| `TestExplorer_D37fAuthorityHtmlOverlay_CreatesOverlayInsideCyMount` | `_installHtmlCardOverlay` appends `.cytoscape-poc-html-overlay` to `mount` (= `.cytoscape-poc-mount`) |
| `TestExplorer_D37fAuthorityHtmlOverlay_RunsByDefault` | Theme-gate retired: `_renderPayload` calls `_installHtmlCardOverlay(_cy, mount, elements)` unconditionally |
| `TestExplorer_D37fAuthorityHtmlOverlay_OverlayIsPointerPassiveAndNonClipping` | Overlay CSS: `pointer-events: none`; NO `overflow: hidden`; `transform-origin: top left` |
| `TestExplorer_D37fAuthorityHtmlOverlay_PreservesAuthorityCardDesign` | Per-kind selectors + sizes still present (verbatim from existing CSS) |
| `TestExplorer_D37fAuthorityHtmlOverlay_UsesTwoTierTransform` | `_syncLayer` body: `_cy.pan()`, `_cy.zoom()`, `'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')'`, `transformOrigin = 'top left'`. `_syncCards` body: `n.position()`, `'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)'` |
| `TestExplorer_D37fAuthorityHtmlOverlay_BindsLayerSyncEvents` | `var LAYER_SYNC_EVENTS = 'pan zoom render resize'`; `_cy.on(LAYER_SYNC_EVENTS, _syncLayerBound)` |
| `TestExplorer_D37fAuthorityHtmlOverlay_BindsCardSyncEvents` | `var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'`; `_cy.on(CARDS_SYNC_EVENTS, _syncCardsBound)` |
| `TestExplorer_D37fAuthorityHtmlOverlay_LayerSyncIsO1` | `_syncLayer` body MUST NOT contain `_htmlCardsByKey` iteration (no Object.keys / for loop over cards) |
| `TestExplorer_D37fAuthorityHtmlOverlay_CardSyncUsesModelPositionOnly` | `_syncCards` body MUST NOT contain `renderedPosition` or `scale(` (layer owns projection) |
| `TestExplorer_D37fAuthorityHtmlOverlay_MirrorsSelectedState` | `_syncCards` reads `n.selected()` and toggles `.selected` class |
| `TestExplorer_D37fAuthorityHtmlOverlay_DestroyRemovesOwnedDomAndListeners` | `_destroyHtmlCardOverlay` body: cancels both rAFs, `_cy.off(LAYER_SYNC_EVENTS, …)`, `_cy.off(CARDS_SYNC_EVENTS, …)`, removes overlay element |
| `TestExplorer_D37fAuthorityHtmlOverlay_DoesNotBreakCytoscapeInteraction` | Card CSS: `pointer-events: none`; overlay CSS: `pointer-events: none` |
| `TestExplorer_D37fAuthorityHtmlOverlay_HostContractPreserved` | `graph-viewport.js` executable code remains renderer-neutral; `.midas-graph-viewport { overflow: hidden }` strategic clip preserved; `.context-cy-spike-overlay` remains non-clipping |
| `TestExplorer_D37f_D37bD37dContractsPreserved` | D37b register/activateById/aria-label/panel bridge + D37d mount `position: absolute; inset: 0` preserved |
| `TestExplorer_D37fAuthority_NoLegacyOneTierFallback` | `_installHtmlCardOverlay` body MUST NOT contain `renderedPosition` (forbids regression to the pre-D37f one-tier model) |

### Tests that must NOT regress

Foundation regression suite must still pass:
- D34i Context two-tier (the canonical model D37f mirrors).
- D35e Context overlay non-clipping.
- D35f host-owned renderer identity.
- D35g registry.
- D36a Knowledge shell.
- D37b Authority production renderer id.
- D37d Authority mount positioning.

---

## 16. Implementation tranche plan — D37f

### Files to modify

| File | Change |
|---|---|
| `internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js` | (a) Add `LAYER_SYNC_EVENTS` / `CARDS_SYNC_EVENTS` / `PROJECTION_MODEL` constants. (b) Add `_syncLayer()` and `_syncCards()` functions (lifted from Context spike with `_overlayEl` → `_htmlOverlayEl` and `_cardsByKey` → `_htmlCardsByKey`). (c) Add `_syncLayerRaf`, `_syncCardsRaf`, `_syncLayerBound`, `_syncCardsBound` module state. (d) Replace `_installHtmlCardOverlay`'s event binding with two-tier binding. (e) Replace `_updateHtmlCardOverlay` with `_syncCards`-style body (already largely there; remove the one-tier renderedPosition path, swap for `n.position()` + `translate(-50%, -50%)`). (f) Update `_destroyHtmlCardOverlay` to unbind both tiers + cancel both rAFs. (g) Remove theme-gate from `_installHtmlCardOverlay`'s call site in `_renderPayload`. (h) Mirror `n.selected()` onto `.selected` card class in `_syncCards`. |
| `internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css` | (a) `.cytoscape-poc-html-overlay`: remove `overflow: hidden`; add `transform-origin: top left`. (b) Drop `[data-cy-theme="html-card"]` qualifier from all HTML-card rules so they apply under default Authority activation. (c) Add a `.selected` rule on `.cytoscape-poc-html-card` for selected-state visual (border thicker / accent colour). |

### Files NOT to modify

- `graph-viewport.js`
- backend (`internal/graph/authority/*`, `internal/httpapi/authority_graph_handler.go`)
- OpenAPI spec
- Context Cytoscape spike (its own D35e tests pin its current shape)
- Knowledge Graph shell
- `cytoscape-html-overlay.js` (D34a generic helper — leave in
  place, opt-in gated, harmless)
- `authority-graph-view.js` (legacy native renderer; D37b kept as
  fallback)
- Inspector / Workbench / panels (D37b bridge already wires these
  through the cached projection)

### Implementation order (suggested PR ordering inside D37f)

1. **CSS change** first (drop `overflow: hidden`, add `transform-origin: top left`, drop `[data-cy-theme="…"]` qualifier). If JS is unchanged at this point, default theme is still `'classic'` so visuals are unaffected — but the moment the JS install path fires unconditionally, cards land correctly.
2. **Add constants + two-tier sync functions** in the JS without yet changing the install call site. The new functions are additive.
3. **Replace `_updateHtmlCardOverlay` body** with the two-tier `_syncCards` body. Replace the single `cy.on('render pan zoom position', …)` binding with the two `cy.on(LAYER_SYNC_EVENTS / CARDS_SYNC_EVENTS, …)` bindings.
4. **Remove the theme-gate** in `_renderPayload` so install fires unconditionally.
5. **Update `_destroyHtmlCardOverlay`** to unbind both tiers + cancel both rAFs.
6. **Add `n.selected()` mirror** in `_syncCards`.
7. **Run the test suite**, then iterate on the new D37f tests.

### Rollback considerations

If D37f introduces a visible regression in browser, the rollback
is small:
- Revert the CSS rules (re-add `overflow: hidden`, re-add `[data-cy-theme="html-card"]` qualifier).
- Restore the theme-gate in `_renderPayload`'s `_installHtmlCardOverlay` call.
- Revert the JS two-tier replacement; keep the original
  `_updateHtmlCardOverlay` one-tier body.

The change is narrow enough that a single commit can be reverted
cleanly.

---

## 17. Risks and open questions

### Risks

| Risk | Severity | Mitigation |
|---|---|---|
| **Card-vs-cy-node visual stacking**: when both the cy node AND the HTML card render, they may visually compete. | MEDIUM | Adopt the existing `'html-card'` theme descriptor's "small and faint" cy node style (`rootOverlayOpacity: 0`, low-opacity node fill) by default. Or use the D34a helper's `_dimNativeNodes` approach. Recommend setting cy node fill opacity low so the HTML card is the visible layer. |
| **`cy.fit()` padding mismatch**: cy fits to its NODE bounding boxes; cards have their own footprint (240×96px). If cy nodes are smaller than cards, `cy.fit()` may leave cards spilling outside the viewport edges. | MEDIUM | Ensure cy node footprint (set via theme descriptor `nodeW` / `nodeH`) matches the card footprint 240×96. The existing `'html-card'` theme descriptor at [authority-cytoscape-poc.js:1925-1937](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1937) already sets `nodeW: 240, nodeH: 96` — D37f must apply this footprint regardless of the URL theme query param. |
| **`?cyTheme=other` interactions**: users may pin a non-html-card theme via URL flag and find the HTML cards still render on top. | LOW | Acceptable. The URL theme is for engineering exploration; production users won't set it. The cards render correctly regardless. |
| **Selected-state CSS visual not yet defined**: D37f mirrors `.selected` onto cards, but no CSS rule consumes the class today. | LOW | D37f's CSS change should add a `.selected` rule (e.g. thicker border or accent ring). |
| **Cytoscape font-family warning** (unrelated to D37f, observed in browser today) | LOW | Out of D37f scope per the brief. |
| **Render-frequency on large graphs**: at depth=5 with many surfaces, card count could climb. | LOW | Two-tier model collapses pan/zoom to O(1); per-card writes are cheap; rAF coalescing prevents bursts. If a future tranche profiles a real problem, consider virtualisation. |

### Open questions (UNKNOWNs requiring browser confirmation)

| # | Question | Browser check |
|---|---|---|
| U1 | Are the cy nodes underneath HTML cards visually distracting in default theme? | After D37f lands, paint Authority with a populated service and inspect visual stacking. |
| U2 | Does `cy.fit()` land correctly when cards are 240×96 but cy nodes default to a different size? | Browser inspect: cards should not spill beyond viewport edges. |
| U3 | Does node drag work cleanly with two-tier sync (cards follow during drag)? | Drag a node; cards should follow per-frame. |
| U4 | Does the workbench / diagnostics / posture panel bridge still fire correctly after D37f? | After D37f, check that the D37b bridge calls still happen (they're in `_renderPayload` after `_settleFit`). |
| U5 | Does `.selected` mirror reach the cards on cy tap? | Click a node; the underlying cy node selects (existing behaviour) AND the corresponding card should gain `.selected` class. |

---

## 18. Final recommendation

**Proceed with D37f as planned.**

The Authority HTML card overlay code is 90% complete. The
remaining 10% is:

1. **Migrating to the proven D34i two-tier transform model**
   (lifted verbatim from the Context spike, where it's pinned by
   tests).
2. **Removing the latent disappearing-card clip bug** (one CSS
   line).
3. **Dropping the theme-gate** so cards render by default.
4. **Mirroring selected state** onto the card class.

Risk is **LOW–MEDIUM**. The implementation is small (under ~150
LOC), the model is well-understood, and the rollback is clean.
The main UX-side validation needed (U1–U5) is straightforward in
the browser after the change lands.

**Do not** redesign the card DOM. **Do not** rename PoC classes.
**Do not** change the projection or backend. **Do not** touch
graph-viewport.js. **Do not** introduce a new theme. **Do not**
touch the legacy native renderer.

The next tranche is:

### D37f — Authority HTML-Card Overlay Implementation

- Migrate Authority overlay to D34i two-tier model
- Drop theme-gate (HTML cards = default Authority visual)
- Fix overlay clipping (remove `overflow: hidden`, add `transform-origin: top left`)
- Mirror selected state onto cards
- Add D37f test suite
- Browser smoke validation against U1–U5
