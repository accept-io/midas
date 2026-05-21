# D37g — Authority HTML-Card Design Recovery Assessment

> **Status:** Read-only assessment. No source, CSS, or test files were
> modified in this tranche. The only artifact produced is this report.
>
> **Scope:** Discover where the richer Authority card / icon design lives
> in the codebase (from earlier PoC work) and produce a precise plan for
> applying it to the current D37f minimal HTML overlay cards in a
> following implementation tranche (D37h).

---

## 1. Executive summary

The D37f HTML-card overlay is **production-default** and behaviourally
correct (two-tier projection, selected-state mirror, no clipping, O(1)
pan/zoom). What it is **missing** is the per-kind iconography of the
earlier richer Authority card design.

Investigation findings:

1. **The previous icon set was found, in full.** Seven Authority kinds
   are mapped to seven vendored Lucide SVGs through a stable two-layer
   contract: `_AUTHORITY_KIND_ICON_KEYS` in the Authority renderer maps
   `kind` → MIDAS-facing key, and the MIDAS Icon Registry maps each key
   → its vendored Lucide source. Both `inlineSvg(name, opts)` (for HTML
   embedding) and `cytoscapeDataURI(name, opts)` (for Cytoscape
   `background-image`) are already implemented, sanitised, and exposed
   on `window.MIDASExplorerIcons`.
2. **The richer card layout (icon + header + body) is not currently
   embodied in any HTML overlay implementation.** Icons are wired into
   Cytoscape *node* styling (`background-image` per theme), not into the
   HTML overlay cards. The richer card design exists as a *design
   intent* (per-kind palette, type chip + title + status, vendored icon
   per kind) distributed across the icon registry, the per-kind CSS
   border colours, and the current minimal card DOM — but no single
   prior implementation assembles all three.
3. **No backend changes are required.** The current per-card payload
   (`d.id`, `d.kind`, `d.label`, `d.isRoot`, `d.raw.<kind>.status`)
   already carries everything D37h needs. The icon is *derived* from
   `d.kind` via the existing registry — it is not new data.
4. **D37h is a small, contained change.** The recommended approach
   modifies `_buildHtmlCard` (one function, ~37 lines today) to emit an
   `<svg>` icon next to the kind chip inside a header row, and adds
   ~3 CSS rules for header / icon sizing within the existing 240×96
   card footprint. The two-tier transform, install / destroy lifecycle,
   selected-state mirror, per-kind border colour, root halo, status row,
   data attributes, and 240×96 footprint are all preserved unchanged.

The gap between "what we have" and "what was intended" is therefore one
function body plus a handful of CSS rules — the icon plumbing is
already done.

---

## 2. Current D37f card inventory

Authoritative source:
[authority-cytoscape-poc.js:1925-1962](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1962)
and the per-kind CSS in
[authority-cytoscape-poc.css:100-192](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L100-L192).

### 2.1 What `_buildHtmlCard(d)` emits today

```html
<article class="cytoscape-poc-html-card"
         data-node-id="…"
         data-kind="…"
         [data-root="true"]>
  <span class="cytoscape-poc-html-card-kind">Business Service</span>
  <div  class="cytoscape-poc-html-card-title">Trade Approval</div>
  <!-- emitted only when d.raw.<kind>.status (or agent.operational_state) is truthy -->
  <div  class="cytoscape-poc-html-card-status">ACTIVE</div>
</article>
```

### 2.2 What the D37f CSS gives the card

| Selector | Effect |
|---|---|
| `.cytoscape-poc-html-overlay` | `position: absolute; inset: 0; pointer-events: none; z-index: 5; transform-origin: top left` (layer tier write target) |
| `.cytoscape-poc-html-card` | 240×96, `display: flex; flex-direction: column; gap: 4px`, dark surface fill, 1px outline-variant border, 6px radius, soft shadow, `pointer-events: none`, `overflow: hidden` |
| `.cytoscape-poc-html-card-kind` | 9px uppercase letter-spaced label |
| `.cytoscape-poc-html-card-title` | 13px / 600 weight, 2-line clamp, ellipsis |
| `.cytoscape-poc-html-card-status` | 10px uppercase label |
| `…[data-kind="…"]` (×7) | Per-kind border colour (and `border-style: dashed` for fail-mode + escalation) |
| `…[data-root="true"]` | Primary-accent halo around the root node card |
| `…[data-active-renderer="authority"] …card.selected` | Selected-state accent shadow (D37f addition) |

### 2.3 What the card visually communicates today

- **Kind** — via the small all-caps text chip *and* the per-kind border
  colour.
- **Identity** — via the title (label or id fallback).
- **Status** — via the optional status line (only when data carries it).
- **Root-ness** — via the primary-accent halo.
- **Selection** — via the D37f accent shadow.

### 2.4 What is missing relative to the original card design intent

- **No icon glyph.** The per-kind iconography that the cy *node*
  styling already uses is not present on the HTML card. This is the
  central D37g gap.
- **No structural header.** Kind chip, title, status are stacked
  vertically without a designated header row, so when an icon is added
  it cannot simply slot in without a small layout change.

Everything else from the original design intent (per-kind palette,
root halo, type chip, title, status, footprint) is already present.

---

## 3. Previous icon set inventory

**Result: FOUND.** The previous icon set is fully defined and reachable
from the Authority renderer today. Nothing needs to be rebuilt.

### 3.1 Renderer-side mapping

[authority-cytoscape-poc.js:200-208](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L200-L208)
declares the `kind → registry-key` mapping:

```js
var _AUTHORITY_KIND_ICON_KEYS = {
  business_service:  'authorityBusinessService',
  decision_surface:  'authorityDecisionSurface',
  authority_profile: 'authorityProfile',
  authority_grant:   'authorityGrant',
  agent:             'authorityAgent',
  fail_mode_policy:  'authorityFailModePolicy',
  escalation_target: 'authorityEscalationTarget',
};
```

[authority-cytoscape-poc.js:226-233](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L226-L233)
declares the safe-fallback accessor used by the cy node styling today:

```js
function _iconForKind(kind, stroke) {
  var icons = window.MIDASExplorerIcons;
  var key = _AUTHORITY_KIND_ICON_KEYS[kind];
  if (!icons || !key || typeof icons.has !== 'function' || !icons.has(key)) {
    return '';
  }
  return icons.cytoscapeDataURI(key, { stroke: stroke || '#e2e8f0' });
}
```

`_iconForKind` is consumed today as `background-image` on cy nodes —
see the per-kind theme entries at
[authority-cytoscape-poc.js:2334, 2551, 2569, 2589, 2608, 2626, 2653, 2675, 2919](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L2334).
This proves the registry is already loaded and the mapping is already
exercised in production rendering.

### 3.2 Registry-side mapping

[midas-icons.js:99-107](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L99-L107)
maps each Authority key to a vendored Lucide source:

| Registry key | Lucide source | Vendored SVG |
|---|---|---|
| `authorityBusinessService`  | `building-2`         | `assets/icons/lucide/building-2.svg` |
| `authorityDecisionSurface`  | `workflow`           | `assets/icons/lucide/workflow.svg` |
| `authorityProfile`          | `shield-check`       | `assets/icons/lucide/shield-check.svg` |
| `authorityGrant`            | `file-check-2`       | `assets/icons/lucide/file-check-2.svg` |
| `authorityAgent`            | `bot`                | `assets/icons/lucide/bot.svg` |
| `authorityFailModePolicy`   | `triangle-alert`     | `assets/icons/lucide/triangle-alert.svg` |
| `authorityEscalationTarget` | `arrow-up-from-line` | `assets/icons/lucide/arrow-up-from-line.svg` |

### 3.3 Public registry surface

Exposed on `window.MIDASExplorerIcons` at
[midas-icons.js:331-339](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L331):

- `has(name) → bool` — guard for unknown names.
- `inlineSvg(name, opts)` — returns a DOM-insertable `<svg>` string
  ([midas-icons.js:258-291](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L258-L291));
  configurable `size`, `stroke`, `strokeWidth`, `className`, `title`,
  `ariaHidden` (defaults: 24, `currentColor`, 2, no class, no title,
  `aria-hidden="true"`). Returns `''` for unknown names — never throws.
- `cytoscapeDataURI(name, opts)` — returns a `data:image/svg+xml;utf8,…`
  URI for Cytoscape `background-image` (already used by `_iconForKind`).

**`inlineSvg` is the helper D37h should use for HTML card embedding.**
The default `currentColor` stroke is exactly what we want inside HTML
cards — the icon will inherit the card's text colour, automatically
adapting to per-kind border colour cues if we choose to coordinate
them later. (For cy nodes, `cytoscapeDataURI` substitutes a concrete
neutral colour because Cytoscape SVG `background-image` does not
propagate `currentColor`.)

---

## 4. Previous richer card design inventory

**Result: PARTIALLY FOUND — distributed, not consolidated.** No single
prior HTML overlay implementation assembles "icon + header + body" the
way the design intent calls for. The pieces of the richer design live
in three places, each of which is already merged and stable:

1. **Per-kind iconography** — lives in the MIDAS Icon Registry
   (§3) and is consumed by Cytoscape *node* styling (`background-image`)
   inside the `object-card`, `object-card-v2`, `object-tile-v3`, and
   related theme descriptors in `authority-cytoscape-poc.js`
   (lines 2334, 2551–2675, 2919). These themes paint the *cy node*
   (now sized 240×96 to match the HTML card footprint), not the HTML
   overlay card. As of D37f the HTML overlay is the default visual, so
   the cy node sits underneath the HTML card and the icon it carries is
   not surfaced to the operator.

2. **Per-kind palette** — already lives in CSS at
   [authority-cytoscape-poc.css:158-180](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L158-L180)
   as per-kind border colour rules on `.cytoscape-poc-html-card`.
   This is the only piece of the richer design that the HTML card
   already honours.

3. **Card DOM scaffolding** — lives in `_buildHtmlCard`
   ([authority-cytoscape-poc.js:1925-1962](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1962)).
   Today it emits kind chip + title + optional status. It needs to
   grow a header row that hosts both the icon and the kind chip.

There is **no historical HTML overlay code** in the tree that already
embeds an icon inside a card. We are not "restoring" lost code; we are
finishing the visual identity by routing the existing icon set through
the existing card DOM.

### 4.1 Why this is recovery and not invention

- The icon *choice per kind* is not being designed in D37h — it was
  decided when the registry was authored (D33a-spike-2g-impl-1) and is
  already pinned by tests and consumed by the cy node styling.
- The icon *colour and sizing convention* (24px viewBox, 2px stroke,
  `currentColor`) is the registry's documented default — we are using
  the default, not inventing one.
- The *placement* (header row, leading icon, type chip beside it) is
  the only new decision, and it is a one-time micro-layout inside an
  already-sized 240×96 card.

In other words: D37h **applies an existing icon contract to an existing
DOM**, with one small structural CSS rule. No new icons, no new
colours, no new sizes, no new files.

---

## 5. Current-vs-intended design gap table

| Concern | D37f (current) | Intended (post-D37h) | Gap size |
|---|---|---|---|
| Per-kind border colour | ✅ Present (CSS §4.1 palette) | Same | — |
| Type chip | ✅ Present (`-kind` span) | Same, but moved inside a header row | trivial |
| Title | ✅ Present, 2-line clamp | Same | — |
| Status row (data-conditional) | ✅ Present | Same | — |
| Root halo | ✅ Present (`data-root="true"`) | Same | — |
| Selected-state mirror | ✅ Present (D37f `.selected`) | Same | — |
| 240×96 footprint | ✅ Present | Same | — |
| Per-kind icon glyph | ❌ Absent | ✅ Inline SVG via `MIDASExplorerIcons.inlineSvg(_AUTHORITY_KIND_ICON_KEYS[kind])` | **Single new DOM element + ~3 CSS rules** |
| Header row (icon + chip) | ❌ Absent (chip + title are siblings of the column flex) | ✅ Header row holds icon + chip; title and status remain below | **One added wrapper div + flex CSS** |
| Two-tier transform contract | ✅ Present (D37f) | Unchanged | — |
| Install / destroy lifecycle | ✅ Present (D37f) | Unchanged | — |
| Data attributes (`data-node-id`, `data-kind`, `data-root`) | ✅ Present | Unchanged | — |
| Pointer-passive overlay + cards | ✅ Present | Unchanged | — |
| Backend payload | ✅ Carries `kind`, `label`, `raw.*.status`, `isRoot` | Unchanged | — |
| `_iconForKind` (cy node background-image) | ✅ Wired | Unchanged (continues to style cy nodes underneath) | — |

The only D37h-load-bearing rows are the two `❌` entries. Everything
else stays exactly as D37f shipped it.

---

## 6. Data availability review

The per-card payload `d` already carries everything D37h needs. No
backend, projection, or contract changes are required.

| Field used by enriched card | Source | Present today? |
|---|---|---|
| `d.kind` | Set by `_renderPayload` from server projection | ✅ used at `_buildHtmlCard:1929` |
| `d.label` | Same | ✅ used at `_buildHtmlCard:1939` |
| `d.id` | Same | ✅ used at `_buildHtmlCard:1928, 1939` |
| `d.isRoot` | Same | ✅ used at `_buildHtmlCard:1930` |
| `d.raw.<kind>.status` / `agent.operational_state` | Same | ✅ used at `_buildHtmlCard:1947-1959` |
| Icon for the kind | Derived: `_AUTHORITY_KIND_ICON_KEYS[d.kind]` → `MIDASExplorerIcons.inlineSvg(key)` | ✅ both halves of the derivation already exist; the lookup is pure-client |

**Result: no projection change, no contract bump, no payload change.**
The icon is a pure client-side derivation from `d.kind`.

---

## 7. Recommended implementation approach (for D37h)

> The rest of this section is a *plan*. No code in this tranche is
> changed.

### 7.1 Single function-level change

Modify `_buildHtmlCard(d)` at
[authority-cytoscape-poc.js:1925-1962](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1962)
to:

1. Build an inline SVG icon for `d.kind` via:
   ```js
   var iconKey = _AUTHORITY_KIND_ICON_KEYS[d.kind || ''];
   var iconSvg = '';
   if (iconKey && window.MIDASExplorerIcons &&
       typeof window.MIDASExplorerIcons.inlineSvg === 'function') {
     iconSvg = window.MIDASExplorerIcons.inlineSvg(iconKey, {
       size: 18,
       className: 'cytoscape-poc-html-card-icon',
       title: _nodeTypeLabel(d.kind || ''),  // exposes kind to screen readers
     });
   }
   ```
   Use `inlineSvg` (not `cytoscapeDataURI`) — HTML embedding inherits
   `currentColor` and supports a real `<title>` element.

2. Wrap the existing kind chip and the new icon in a header row:
   ```js
   var headerEl = document.createElement('div');
   headerEl.className = 'cytoscape-poc-html-card-header';
   if (iconSvg) {
     var iconWrap = document.createElement('span');
     iconWrap.className = 'cytoscape-poc-html-card-icon-wrap';
     iconWrap.innerHTML = iconSvg;
     headerEl.appendChild(iconWrap);
   }
   headerEl.appendChild(kindEl);    // pre-existing -kind span moves into header
   card.appendChild(headerEl);
   card.appendChild(titleEl);
   // status row append unchanged
   ```

3. Preserve every existing property: `className`, `data-node-id`,
   `data-kind`, `data-root`, title clamp, status row, overall card
   layout. No `_install*` / `_destroy*` / `_syncLayer` / `_syncCards`
   change is required because the DOM nodes inside an `<article>` are
   irrelevant to the two-tier transform — the layer tier writes onto
   the overlay container, the cards tier writes onto the `<article>`
   element itself. Adding children to the article costs nothing in the
   sync path.

### 7.2 Why this approach is safe

- **No projection model change.** `PROJECTION_MODEL =
  'layer-pan-zoom-card-model-position'` and both `LAYER_SYNC_EVENTS` /
  `CARDS_SYNC_EVENTS` lists are untouched.
- **No lifecycle change.** `_installHtmlCardOverlay` and
  `_destroyHtmlCardOverlay` are untouched. New DOM nodes inside the
  article are destroyed with the article via the existing teardown.
- **No data contract change.** Backend, projection, and per-card payload
  are unchanged.
- **No registry change.** `MIDASExplorerIcons` is consumed via its
  already-public surface (`has`, `inlineSvg`). No new icons added.
- **No regression risk for the cy node styling.** `_iconForKind`
  continues to feed cy-node `background-image` as before. The HTML
  card icon is an independent consumer of the same registry.
- **Graceful degradation preserved.** If `MIDASExplorerIcons` is
  unavailable or returns `''` for the key, the header row simply has
  no icon — the card still renders with chip + title + status as
  today. (Same robustness pattern as `_iconForKind`.)

### 7.3 What the new DOM looks like

```html
<article class="cytoscape-poc-html-card"
         data-node-id="…" data-kind="business_service"
         [data-root="true"] [.selected]>
  <div class="cytoscape-poc-html-card-header">
    <span class="cytoscape-poc-html-card-icon-wrap">
      <svg class="cytoscape-poc-html-card-icon" …>…building-2…</svg>
    </span>
    <span class="cytoscape-poc-html-card-kind">Business Service</span>
  </div>
  <div class="cytoscape-poc-html-card-title">Trade Approval</div>
  <!-- status row unchanged, still data-conditional -->
  <div class="cytoscape-poc-html-card-status">ACTIVE</div>
</article>
```

---

## 8. CSS / footprint plan

The card footprint **stays at 240×96**. Cytoscape node geometry is
sized to match (D37f explicitly chose `html-card` as `DEFAULT_THEME` to
align cy node footprint with HTML card footprint for `cy.fit()`
correctness) — changing the footprint would invalidate that alignment.

### 8.1 New CSS rules required

Add to
[authority-cytoscape-poc.css](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css)
inside the existing `.midas-graph-viewport[data-active-renderer="authority"]`
scope used by all current HTML-card rules:

```css
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-card-header {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
}
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-card-icon-wrap {
  display: inline-flex;
  flex: 0 0 auto;
  width: 18px;
  height: 18px;
  color: var(--on-surface-variant, #94a3b8);
}
.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-card-icon {
  width: 18px;
  height: 18px;
  display: block;
}
```

### 8.2 Existing rules that change behaviour without being edited

- `.cytoscape-poc-html-card` is already `display: flex;
  flex-direction: column; gap: 4px`, so the new header row slots in as
  the first column child without changing the card flexbox model.
- `.cytoscape-poc-html-card-kind` already styles the chip exactly as
  desired; moving it into the header row does not require any rule
  change. Its inline-block / 9px / uppercase / letter-spaced styling
  continues to apply.
- `.cytoscape-poc-html-card-title` already uses `flex: 1 1 auto`, so
  the title still gets the remaining vertical space below the header
  and above the optional status row.

### 8.3 Why 18px

- The card body is 96px tall with `padding: 10px 12px`, so the
  content height is 76px.
- 18px icon + 4px column gap + 13px / 600 title (two lines ≈ 34px) +
  4px gap + 10px status leaves comfortable headroom and matches the
  visual weight of a small chip without dominating the title.
- 18px is also a clean multiple of the registry's 24px viewBox at 75%
  scale — Lucide strokes render crisply at this size with `stroke-width: 2`.

### 8.4 Optional per-kind icon colour coordination (deferred)

The per-kind border colours (CSS §4.1, lines 158-180) carry the kind
identity by themselves. If we wanted the icon to colour-match the
border, the icon-wrap rules above already use `color` and the SVG
inherits `currentColor`, so a per-kind override of the form

```css
.midas-graph-viewport[…] .cytoscape-poc-html-card[data-kind="business_service"]
  .cytoscape-poc-html-card-icon-wrap { color: var(--primary, #7aa2ff); }
```

would suffice. **Defer this to D37i** unless review specifically asks
for it — the neutral-stroke version reads cleanly and avoids over-
saturating the card with kind colour.

---

## 9. Risks and open questions

### 9.1 Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Icon registry script not loaded before Authority renderer initialises | LOW | Already mitigated. `_iconForKind` uses the same defensive guard and is exercised in production today. The new code reuses the same guard pattern (`has` + function-type checks) and degrades to no-icon — never throws. |
| Icon overflows the card on narrow titles | NONE | 18px icon inside a row with `gap: 6px` next to a 9px chip uses ~80px of horizontal space out of 216px content width. Plenty of slack for any chip length we emit. |
| Two-line title clamp fights for vertical space against the new header row | LOW | Header row is 18px tall + 4px gap = 22px. Title remains 13px × 2 lines = 34px. Status (when present) is 10px. Total ≈ 70px inside 76px content height. Already validated against the existing card body padding. If a title legitimately needs three lines it would already overflow the 2-line clamp today — D37h does not change that. |
| Per-kind icon colour clashes with per-kind border colour | LOW | Default uses neutral `--on-surface-variant` so the border carries kind colour and the icon stays calm. The optional coordination in §8.4 is deferred. |
| Adding inline SVG strings via `innerHTML` is unsafe | NONE | `MIDASExplorerIcons.inlineSvg` is the *only* path used. Its output is hard-coded from vendored SVGs + a sanitised `opts.title` (escapes via `escapeText`) + sanitised numeric / class-name fields. There is no user-controlled data on the icon path. |
| Test asset-text pins break because card DOM grew | EXPECTED | Tests that pin `_buildHtmlCard` body must be updated to expect the new header element. This is exactly the bounded test cost the brief flags. |

### 9.2 Open questions

1. **Should the icon size scale with zoom?** No. The layer-tier
   transform already scales the entire overlay (the icon along with
   it). Setting icon size in px on the card is the correct
   "model-space" choice — the layer scale projects it.
2. **Should we surface kind via `aria-label` on the article, not on
   the icon?** The proposed approach gives the icon a `<title>` (via
   `opts.title`) — this is the standard SVG screen-reader hook and the
   article already carries `data-kind`. An additional `aria-label` on
   the article would be redundant. If a follow-up accessibility review
   wants both, it is one extra line.
3. **Should we add an icon for the "unknown kind" case?** The current
   safe-fallback returns `''`. Recommendation: keep that. An unknown
   kind already loses the per-kind border colour treatment, so the
   missing icon is internally consistent with the rest of the design.
4. **Should the header row also surface the root indicator?** No.
   `data-root="true"` already drives a halo around the entire card.
   Duplicating it in the header would clutter the small footprint.

---

## 10. Test plan (for D37h)

These tests should land in `internal/httpapi/explorer_d37h_test.go`
(suggested file name; consistent with D37f's pattern). All are asset-
text pins against the JS / CSS bodies — no browser harness required.

### 10.1 New tests (positive — what D37h adds)

1. `TestExplorer_D37h_HtmlCardHasIconElement` — pins that the
   `_buildHtmlCard` body contains a call to
   `MIDASExplorerIcons.inlineSvg(` and emits an element with class
   `cytoscape-poc-html-card-icon-wrap`.
2. `TestExplorer_D37h_HtmlCardIconUsesAuthorityKindKeyMap` — pins that
   the icon key passed to `inlineSvg` is looked up from
   `_AUTHORITY_KIND_ICON_KEYS[…]`. (Guards against the icon being
   hard-coded per kind or sourced from a fresh map.)
3. `TestExplorer_D37h_HtmlCardEmitsHeaderRow` — pins
   `cytoscape-poc-html-card-header` class on a new wrapping div, and
   confirms the kind chip and icon wrap are inside it.
4. `TestExplorer_D37h_HtmlCardKindChipMovedIntoHeader` — verifies the
   `-kind` span is appended to the header element, not directly to the
   card. Stops a future refactor from accidentally re-flattening the
   header.
5. `TestExplorer_D37h_HtmlCardIconUsesInlineSvgNotDataUri` — pins that
   D37h calls `inlineSvg` (not `cytoscapeDataURI`); the HTML embedding
   path is the right one for `currentColor` and `<title>`.
6. `TestExplorer_D37h_HtmlCardIconCarriesAccessibleTitle` — pins that
   the `inlineSvg` call passes `title:` derived from
   `_nodeTypeLabel(d.kind)`.
7. `TestExplorer_D37h_HtmlCardIconDegradesGracefullyWhenRegistryAbsent`
   — pins the guard sequence `MIDASExplorerIcons &&
   typeof MIDASExplorerIcons.inlineSvg === 'function'` (or equivalent)
   before calling, so a missing registry yields a card without an icon
   instead of an exception.
8. `TestExplorer_D37h_HtmlCardCssDefinesHeaderRowStyles` — pins the
   three new CSS rules (`-header`, `-icon-wrap`, `-icon`).
9. `TestExplorer_D37h_HtmlCardFootprintUnchanged` — re-pins
   `width: 240px; height: 96px;` on `.cytoscape-poc-html-card`,
   guarding against accidental footprint growth.

### 10.2 Preservation tests (D37f / D37b / D37d / D35–D36 contracts)

10. `TestExplorer_D37h_D37fContractsPreserved` — re-pins:
    - `DEFAULT_THEME = 'html-card'`
    - `LAYER_SYNC_EVENTS = 'pan zoom render resize'`
    - `CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'`
    - `PROJECTION_MODEL = 'layer-pan-zoom-card-model-position'`
    - `_syncLayer` body still O(1) (no iteration over `_htmlCardsByKey`)
    - `_syncCards` body still uses `n.position()` (not `renderedPosition`)
    - `.selected` class still toggled in `_syncCards`
    - install signature `_installHtmlCardOverlay(cy, mount, elements)` unchanged
11. `TestExplorer_D37h_D37bD37dContractsPreserved` — re-pins:
    - Renderer registers under id `'authority'`
    - `.cytoscape-poc-mount` CSS still uses `position: absolute; inset: 0;`
    - Mount visibility null-guard in `_renderUnavailable`
12. `TestExplorer_D37h_D35D36ContractsPreserved` — re-pins:
    - `.midas-graph-viewport` retains `overflow: hidden` (strategic clip)
    - `data-active-renderer="authority"` attribute still drives the CSS scope
    - Registry contract symbols (`register/unregister/hasRenderer/listRegistered/activateById`) still on host

### 10.3 Negative tests (what D37h must not do)

13. `TestExplorer_D37h_NoBackgroundImageOnHtmlCard` — pins that the
    HTML card CSS does NOT introduce `background-image` (icons live
    in inline SVG, not via background-image on the card; that path is
    reserved for cy node styling).
14. `TestExplorer_D37h_NoNewIconsInRegistry` — confirms the
    `CATALOGUE` Authority entries in `midas-icons.js` are unchanged
    (D37h is a recovery tranche, not an icon-set expansion).
15. `TestExplorer_D37h_NoProjectionContractChange` — confirms backend
    projection types and per-card payload assembly are unchanged.

### 10.4 Tests that will need to be updated (not blockers)

- Existing D37f pins that assert the *exact* shape of `_buildHtmlCard`
  body (kind chip → title → optional status, no header wrapper) will
  need to be relaxed to accept the header row. This is a planned,
  bounded test-relaxation cost — same pattern as the D37f migration.

---

## 11. Browser validation checklist

Manual checks once D37h is wired:

1. Hard-reload Authority lens with no `?cyTheme=` query — confirm
   cards render with the new icon visible in the header row, kind chip
   to its right, title beneath, optional status beneath that.
2. Hover the icon — browser native tooltip should show the kind
   display label (e.g., "Business Service") via the SVG `<title>`.
3. Click a node — selected card gets the D37f accent shadow; icon
   still visible and aligned.
4. Pan + zoom aggressively — icon scales with the rest of the card
   (layer tier transform). No icon "jitter" relative to the chip.
5. Drag a node — card follows; icon and chip remain locked together
   in the header row.
6. Resize the viewport — cards relayout; icon row stays at top.
7. Confirm DevTools shows no `Uncaught` exceptions when the registry
   is temporarily made unavailable (e.g., rename the global in the
   console and re-render). Cards should render without icons; no
   crash.
8. Confirm DevTools shows exactly one `<svg>` per card and the
   expected class names on the new elements.
9. Open the Storybook / asset preview page (if present) to confirm
   the icon registry's source SVGs match what the cards render.
10. With each of the seven Authority kinds present in a fixture,
    confirm seven distinct Lucide glyphs appear and that each matches
    the §3.2 mapping.

---

## 12. Recommended D37h implementation scope

> The user explicitly asked for a recommended next-tranche scope.

**Tranche name:** D37h — Authority HTML-Card Icon Application

**Scope (in):**

- Modify `_buildHtmlCard(d)` in
  `internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js`
  to emit a header row containing an inline SVG icon (via
  `MIDASExplorerIcons.inlineSvg(_AUTHORITY_KIND_ICON_KEYS[d.kind])`)
  alongside the existing kind chip.
- Add three new CSS rules to
  `internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css`
  defining `.cytoscape-poc-html-card-header`,
  `.cytoscape-poc-html-card-icon-wrap`, and
  `.cytoscape-poc-html-card-icon` (header flex row, 18px icon sizing,
  neutral `currentColor` stroke).
- Add new D37h tests (15 listed in §10).
- Relax / update existing D37f tests that pin the exact pre-header
  `_buildHtmlCard` body shape.

**Scope (out):**

- No new icons. No registry changes.
- No projection / contract / backend changes.
- No footprint change (cards stay 240×96; cy node theme stays
  `html-card` default).
- No selected-state or root-halo changes.
- No per-kind icon colour coordination (deferred to D37i if wanted).
- No retirement of the `cytoscape-poc-*` naming debt (still tracked
  separately).
- No work on Knowledge Graph, Context, or other lenses.

**Hard constraints (must hold across D37h):**

- All D37f pins must continue to pass (`PROJECTION_MODEL`,
  `LAYER_SYNC_EVENTS`, `CARDS_SYNC_EVENTS`, `DEFAULT_THEME`,
  `.selected` mirror, install signature).
- All D37b / D37d pins must continue to pass (renderer id `'authority'`,
  mount `position: absolute; inset: 0;`).
- All D35 / D36 GraphViewport pins must continue to pass.
- HTML card must continue to be `pointer-events: none` and Cytoscape
  must continue to own hit-testing.
- The icon path must degrade silently when the registry is absent
  (no exceptions, just no icon).

**Estimated change footprint:** ~30 lines of JS (icon build +
header element), ~15 lines of CSS (three rules), ~15 new tests,
~3–5 existing test relaxations.

---

### Appendix A — File citations referenced in this report

| Concern | File:lines |
|---|---|
| Authority renderer | [authority-cytoscape-poc.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js) |
| Icon kind→key map | [authority-cytoscape-poc.js:200-208](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L200-L208) |
| `_iconForKind` accessor | [authority-cytoscape-poc.js:226-233](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L226-L233) |
| `DEFAULT_THEME` (post-D37f) | [authority-cytoscape-poc.js:108](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L108) |
| Two-tier projection constants | [authority-cytoscape-poc.js:1791-1793](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1791-L1793) |
| `_installHtmlCardOverlay` | [authority-cytoscape-poc.js:1864](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1864) |
| `_buildHtmlCard` (the D37h target) | [authority-cytoscape-poc.js:1925-1962](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1925-L1962) |
| `_updateHtmlCardOverlay` | [authority-cytoscape-poc.js:1969](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1969) |
| `_destroyHtmlCardOverlay` | [authority-cytoscape-poc.js:1975](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1975) |
| MIDAS Icon Registry | [midas-icons.js](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js) |
| Authority catalogue entries | [midas-icons.js:99-107](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L99-L107) |
| `inlineSvg` helper (D37h's icon path) | [midas-icons.js:258-291](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L258-L291) |
| `cytoscapeDataURI` helper (cy node path) | [midas-icons.js:302-327](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L302-L327) |
| `window.MIDASExplorerIcons` surface | [midas-icons.js:331](../../internal/httpapi/explorer/assets/js/icons/midas-icons.js#L331) |
| HTML card CSS (overlay + card + per-kind + selected) | [authority-cytoscape-poc.css:100-192](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L100-L192) |
| Vendored Lucide SVGs | [assets/icons/lucide/](../../internal/httpapi/explorer/assets/icons/lucide/) |
