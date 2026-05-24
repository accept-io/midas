# D38b — Responsive Design Proposal for MIDAS Explorer

**Status:** Proposal. Not yet implemented.
**Predecessor:** D38a-responsive-design-audit-assess (verdict: PARTIAL, HIGH confidence).
**Successors:** D38b-impl-1 through D38b-impl-N (to be tranched per §10).

## 1. Purpose

The D38a audit established that MIDAS Explorer has **partial** responsive support: viewport meta tag is present, 15 desktop-class media queries collapse multi-column layouts at 720–1280 px, and Cytoscape's `ResizeObserver` chain keeps the graph canvas in sync with its container. Beyond that, every established responsive pattern is absent: no phone-class breakpoint, no fluid typography, no breakpoint tokens, no touch hit-target system, no accessibility-adjacent media queries, no mobile-nav pattern, and no viewport-conditional graph styling.

This document proposes the target responsive pattern for MIDAS, sequenced as implementation tranches. It is deliberately a design document, not an implementation plan — concrete CSS, JS, and HTML changes belong to the per-tranche specifications that follow.

The audience for this design is the MIDAS maintainer team (deciding whether to adopt the proposal as written) and the implementer of the first tranche (D38b-impl-1, foundation layer).

## 2. Problem statement (one paragraph)

A bank operator or governance reviewer on a tablet during an incident — or a CNCF reviewer evaluating MIDAS from a phone in the back of a conference room — currently sees a desktop-only Explorer. The shell sidebar collapses below 720 px (a side-effect of one rule in [services.css:480](../../internal/httpapi/explorer/assets/css/services.css)), but the inspector rail stays at 320 px, leaving a 55 px graph canvas on a 375 px-wide phone. Font sizes are fixed at 9–14 px and do not scale with user preference or device. Touch hit targets are below the WCAG 44 × 44 px minimum at every button, tab, and toolbar control except three isolated overrides. Cytoscape node selection on touch is effectively impossible because HTML overlay cards are positioned at desktop dimensions. None of this is hostile to non-desktop devices by intent; it is hostile by neglect.

## 3. Design goals

1. **A bank operator with an iPad can read records, inspect envelopes, navigate the Authority Graph, and resolve escalations** with no functionality loss, even if the layout differs from desktop.
2. **A bank operator with a phone can read records, inspect envelopes, and navigate the catalogue** — the Authority Graph may be reduced to read-only summary form (graph interactions on a 375 px viewport are not a competitive feature; the read-only summary is).
3. **A user with `prefers-reduced-motion` sees no shell or graph animations**; a user with `prefers-color-scheme: light` sees the existing light-mode theme automatically.
4. **All interactive controls meet WCAG 2.5.5 touch target size (44 × 44 px)** on `pointer: coarse` devices, regardless of font size.
5. **The existing desktop experience is preserved byte-identical** at viewports ≥ 1441 px (today's effective design target) and substantially preserved at 1025–1440 px. The proposal is additive, not a desktop redesign.
6. **The proposal is implementable in vertical slices.** Foundation tranche (D38b-impl-1) ships in isolation and provides value; each subsequent tranche builds on stable ground.

## 4. Non-goals

- **A full mobile-first rewrite.** MIDAS is a B2B governance tool; the dominant audience is and will remain desktop. The proposal is desktop-first with progressive degradation for smaller viewports, not the reverse.
- **A mobile companion app.** This proposal is for the web Explorer alone.
- **PWA / offline / install banner.** Out of scope; would be a separate tranche.
- **Touch-specific gestures beyond what Cytoscape provides** (pinch zoom, two-finger pan). MIDAS will not implement custom multi-touch handlers.
- **Phone-class graph editing or detailed inspection.** Graph mode on a 375 px viewport degrades to a read-only summary card list with a "switch to Records view for detail" affordance.
- **Re-theming the existing visual identity.** Colours, dark default, type stack, and component shapes stay as defined in [tokens.css](../../internal/httpapi/explorer/assets/css/tokens.css). The proposal touches sizing, layout, and a11y tokens only.
- **Container queries.** Promising for component-level responsiveness, but baseline browser support for the operator audience is uneven and the proposal is achievable with media queries alone. Container queries are deferred to a future tranche if a specific component justifies them.

## 5. Target device profile

Implementation effort budgets and breakpoint values are sized against this assumed audience distribution:

| Tier | Devices | Expected use cases | Audience share (assumed) |
|---|---|---|---|
| **Wide desktop** (≥ 1441 px) | 27"+ external monitors, ultra-wide | Operator workbench during incident response, graph exploration | 40 % |
| **Desktop** (1025–1440 px) | 13"–15" laptops, dual-monitor secondary | Day-to-day operator use, policy authoring | 45 % |
| **Tablet** (481–1024 px) | iPad portrait/landscape, Surface | Mobile operator triage, governance review, demo | 10 % |
| **Phone** (≤ 480 px) | iPhone, Android phones | Read-only triage during off-hours, alert acknowledgement, CNCF/Manning demo from-anywhere | 5 % |

The share values are assumptions for sizing implementation effort, not telemetry. They drive: **most effort goes to preserving the wide/desktop experience; tablet gets a proper supported layout; phone gets graceful degradation.**

## 6. Foundation layer (D38b-impl-1)

### 6.1 Breakpoint system

Four tiers, defined as CSS custom properties for documentation + JS-side viewport detection, plus matching `@media` literal values (CSS custom properties cannot be used inside `@media` expressions).

```css
:root {
    /* Breakpoint upper bounds (max-width-equivalents for desktop-first
       reflows). Used by JS via window.matchMedia(); CSS @media uses
       literal values matching these tokens by convention. */
    --bp-phone-max:    480px;
    --bp-tablet-max:  1024px;
    --bp-desktop-max: 1440px;
}
```

The existing inline literals (720, 920, 1100, 1280) are migrated as follows:

| Existing | Migrated to | Rationale |
|---|---|---|
| `max-width: 720px` (4 sites) | `max-width: 1024px` | Currently fires at small-laptop; should fire at tablet boundary. |
| `max-width: 920px` (1 site) | `max-width: 1024px` | Same |
| `max-width: 1100px` (6 sites) | `max-width: 1024px` | Same |
| `max-width: 1280px` (3 sites) | `max-width: 1440px` | Currently fires at narrow-desktop; should fire at the desktop/wide boundary. |
| (none) | New `max-width: 480px` rules | Phone-tier rules are net new. |

The migration is **mechanical for 14 of 15 existing queries** (re-target to the standardised values without changing the inside). The 15th — [services.css:480](../../internal/httpapi/explorer/assets/css/services.css) — is the load-bearing shell-collapse rule; it moves from `services.css` to a new `shell-responsive.css` file (per §7) and changes from `max-width: 720px` to `max-width: 1024px`.

### 6.2 Fluid typography scale

Body root font-size becomes `16px` (browser default). All component font-sizes migrate from fixed px to rem.

```css
html { font-size: 16px; }  /* explicit; allows user-agent override */
body {
    font-family: var(--font-body);
    font-size: 0.875rem;    /* 14px equivalent; was the existing default */
    line-height: 1.5;
}
```

Migration table for existing font-size values:

| Current (px) | Proposed (rem) | Equivalent at 16px root |
|---|---|---|
| 9 | 0.5625rem | 9 px |
| 10 | 0.625rem | 10 px |
| 11 | 0.6875rem | 11 px |
| 12 | 0.75rem | 12 px |
| 13 | 0.8125rem | 13 px |
| 14 | 0.875rem | 14 px |
| 16 | 1rem | 16 px |
| 18 | 1.125rem | 18 px |
| 20 | 1.25rem | 20 px |
| 22 | 1.375rem | 22 px |

The migration is **byte-identical visually** at the default root font-size (16px) — every rem value resolves to the same number of pixels it did before. The benefit is that users who increase their browser zoom or set a larger root font-size now see a proportionally larger Explorer.

**`clamp()` for shell sizing only**: body font-size, shell title, and a small number of headlines move to `clamp(min, preferred, max)` so they scale gently across viewport widths. Component-level font-sizes stay at fixed rem.

### 6.3 Touch hit-target standards

A new token expresses the minimum hit-target size and is applied conditionally on `pointer: coarse` devices:

```css
:root {
    --hit-target-min: 44px;  /* WCAG 2.5.5 minimum; Apple HIG 44pt */
}

@media (pointer: coarse) {
    .btn, .tab, .toolbar-button, .menu-item, .icon-button,
    .gmap-camera-control, .canvas-edge-tab-button {
        min-height: var(--hit-target-min);
        min-width:  var(--hit-target-min);
    }
}
```

The `@media (pointer: coarse)` guard ensures **desktop hover users see no visual change**; only touch devices get the enlarged hit areas. The selector list is illustrative; the actual implementation tranche enumerates the canonical interactive selectors and applies the rule once globally.

The three existing isolated 44–48 px rules ([drift.css:176, 310](../../internal/httpapi/explorer/assets/css/drift.css), [governance-map.css:690](../../internal/httpapi/explorer/assets/css/governance-map.css)) become redundant once the global rule lands and are removed in the same tranche.

### 6.4 Accessibility-adjacent media queries

Four media queries are introduced, each in its canonical location (foundation file or component file as appropriate):

```css
/* Foundation file — applies to shell transitions and any global motion. */
@media (prefers-reduced-motion: reduce) {
    *,
    *::before,
    *::after {
        animation-duration: 0.01ms !important;
        animation-iteration-count: 1 !important;
        transition-duration: 0.01ms !important;
        scroll-behavior: auto !important;
    }
}

/* Foundation file — light-mode auto-detection. */
@media (prefers-color-scheme: light) {
    :root:not([data-theme="dark"]) {
        /* Existing :root[data-theme="light"] token overrides applied
           automatically. The :not() guard preserves the existing
           explicit toggle. */
    }
}

/* High-contrast accommodation. */
@media (prefers-contrast: more) {
    :root {
        --outline:        #ffffff;
        --outline-variant: #cccccc;
        /* Increase border weights across components. */
    }
}

/* Touch-device hover guard — see §6.3 and §7. */
@media (hover: none), (pointer: coarse) {
    /* Hover-only affordances (tooltips, hover-revealed actions)
       become tap-revealed; hover styles are suppressed. */
}
```

**Existing light mode behaviour preserved.** The current `:root[data-theme="light"]` opt-in continues to work for users who explicitly toggle. The new `prefers-color-scheme` rule fires only when no explicit theme is set, so a user with OS light-mode gets automatic light theme without losing manual control.

### 6.5 Foundation deliverable

A new file `internal/httpapi/explorer/assets/css/responsive-foundation.css` containing:
- The four breakpoint tokens (§6.1)
- The fluid typography migration applied to body root and shell typography (§6.2)
- The hit-target token + `pointer: coarse` rule (§6.3)
- The four a11y media queries (§6.4)

This file loads after `tokens.css` and before all component CSS so consumers can read its tokens.

D38b-impl-1 lands **only the foundation** — no shell changes, no graph changes. The existing breakpoint literals in 8 CSS files are updated mechanically. Visual diff at desktop is zero; visual diff at ≤ 480 px is "the shell-sidebar still appears at full width because the responsive collapse hasn't moved yet" — that lands in impl-2.

## 7. Explorer shell responsive behaviour (D38b-impl-2)

### 7.1 Sidebar

| Tier | Sidebar treatment |
|---|---|
| Wide (≥ 1441 px) | Full 256 px, always expanded. Existing behaviour. |
| Desktop (1025–1440 px) | Full 256 px, user-collapsible via existing `body.sidebar-collapsed` toggle. Existing behaviour. |
| Tablet (481–1024 px) | Auto-collapsed to 56 px icon rail (icons only). User-expandable via existing toggle, but defaults to collapsed at first load on tablet. |
| Phone (≤ 480 px) | Sidebar becomes an off-canvas drawer. A hamburger button appears in the shell header at top-left. Tapping opens the drawer over the main content with a backdrop scrim; tapping the scrim or a nav item closes it. |

Implementation pattern for the phone-tier off-canvas drawer:

```css
@media (max-width: 480px) {
    .shell-sidebar {
        transform: translateX(-100%);
        transition: transform 0.18s ease-out;
        z-index: 100;
    }
    body.sidebar-drawer-open .shell-sidebar {
        transform: translateX(0);
    }
    .shell-sidebar-backdrop {
        display: none;
    }
    body.sidebar-drawer-open .shell-sidebar-backdrop {
        display: block;
        position: fixed; inset: 0;
        background: rgba(0, 0, 0, 0.5);
        z-index: 99;
    }
}
```

The existing `body.sidebar-collapsed` class continues to drive the desktop/tablet icon-rail; a new `body.sidebar-drawer-open` class drives the phone off-canvas. They are non-conflicting because they apply at different viewport tiers.

A new shell-header element `.shell-sidebar-hamburger` is added, visible only at `≤ 480 px`. It toggles `body.sidebar-drawer-open`. No JS framework is required — a 10-line vanilla handler.

### 7.2 Inspector rail (graph workspace right-side)

This is the load-bearing change. Today the inspector rail consumes 320 px on the right whenever `body.gmap-mode` is active, regardless of viewport. At 375 px viewport this leaves 55 px for the graph.

| Tier | Inspector rail treatment |
|---|---|
| Wide (≥ 1441 px) | Full 320 px, always expanded. Existing behaviour. |
| Desktop (1025–1440 px) | Full 320 px, user-collapsible to 28 px handle via existing `body.inspector-collapsed` toggle. Existing behaviour. |
| Tablet (481–1024 px) | Auto-collapsed to 28 px handle on first load. User-expandable via tap; when expanded, overlays the canvas (no width reservation in shell-main). |
| Phone (≤ 480 px) | Off-canvas drawer pattern, mirroring sidebar. Triggered by the canvas-edge tab system (D37am-aq pattern): tapping a Details/Authority/Evidence tab opens the inspector as a full-screen drawer with a close affordance. |

Implementation pattern: the existing `body.gmap-mode .shell-main { margin-right: var(--inspector-width); }` rule is wrapped in a `@media (min-width: 1025px)` guard so it only applies above the tablet tier. At tablet, the rail floats over the canvas (`position: absolute; right: 0`) — when collapsed it shows the 28 px handle, when expanded it overlays. At phone, the rail becomes a `position: fixed; inset: 0;` drawer.

### 7.3 Header

| Tier | Header treatment |
|---|---|
| Wide / Desktop | Full header with chips, mode toggle, sub-view tabs. Existing behaviour. |
| Tablet | Chips condense to icons-with-tooltip; mode toggle stays. Sub-view tabs may horizontally scroll if needed. |
| Phone | Header height reduces to 48 px; only brand + hamburger + a single "more" overflow button visible; sub-view tabs become a horizontally-scrolling strip below the header. |

### 7.4 Deliverable

A new file `internal/httpapi/explorer/assets/css/shell-responsive.css` (extracted from the rule currently buried in services.css:480) containing all shell-tier responsive rules. The shell-header hamburger element is added to `index.html`. A small `shell-responsive.js` (~50 lines) wires the hamburger toggle and an Escape-key drawer-close handler.

## 8. Graph-specific responsive behaviour (D38b-impl-3)

### 8.1 What stays as-is

Cytoscape's `ResizeObserver` + `cy.resize()` + `cy.fit()` chain is correct and stays. The graph canvas reflows correctly as its container changes size. The existing implementations at [graph-viewport.js:414](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js), [context-cytoscape-renderer.js:158](../../internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-renderer.js), and [authority-cytoscape-poc.js:1659](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js) require no changes.

### 8.2 Viewport-conditional graph styling

A new module `graph-viewport-tier.js` exposes the current viewport tier as a body data-attribute and as a JS-readable signal. Cytoscape stylesheets in `context-cytoscape-renderer.js` and `authority-cytoscape-poc.js` consume the signal to adjust node/edge styling:

| Tier | Node label visibility | Edge label visibility | Node hit-area padding | Camera-control size |
|---|---|---|---|---|
| Wide / Desktop | Always | Always | Default | Default |
| Tablet | Always | On focus / hover only | +6 px | +50 % (visually larger touch buttons) |
| Phone | On focus only | Never | +12 px (sized to meet 44 px minimum) | +100 % |

The hit-area expansion is implemented via Cytoscape's `padding-relative-to: 'data'` or equivalent style — exact mechanism to be confirmed during impl-3 spike.

### 8.3 Camera controls

Toolbar buttons in [authority-cytoscape-toolbar.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js) and the canvas-edge tabs in [authority-canvas-edge-tabs.css](../../internal/httpapi/explorer/assets/css/authority-canvas-edge-tabs.css) gain `@media (pointer: coarse)` rules that lift their hit areas to the 44 × 44 px minimum. These are already covered by the foundation `pointer: coarse` selector list (§6.3) once those selectors are enumerated correctly; no per-component rules are needed.

### 8.4 Phone-tier graph mode

On phone (≤ 480 px), graph mode degrades:
- Graph canvas renders but at reduced fidelity (node labels hidden, edge labels hidden).
- A "summary card list" affordance appears as an alternative to the canvas — a vertically-stacked list of nodes with their kind, status, and key attributes, tappable to open the inspector drawer.
- Selecting a node from the summary list highlights it on the canvas AND opens the inspector drawer.

This is a graceful-degradation pattern. A phone user can still see the structure but is not asked to perform fine-grained graph manipulation.

### 8.5 Fit padding

`cy.fit(undefined, 24)` becomes `cy.fit(undefined, _viewportFitPadding())` where the helper returns:
- 24 at desktop / wide
- 16 at tablet
- 8 at phone

The padding scales because the visible canvas is smaller; a 24 px padding on a 320 px canvas wastes 15 % of horizontal space.

### 8.6 Deliverable

- New module `internal/httpapi/explorer/assets/js/graph/graph-viewport-tier.js` (~80 lines)
- Stylesheet updates inside `context-cytoscape-renderer.js` and `authority-cytoscape-poc.js` to consume the tier signal
- A new `internal/httpapi/explorer/assets/css/graph-responsive.css` with the camera-control + canvas-edge `pointer: coarse` rules (or these become part of the foundation hit-target selector list — implementer decides)
- A small `phone-summary-list.js` (~120 lines) for the phone-tier alternative-to-canvas view

## 9. Migration strategy

The migration is **additive at every step**. Each tranche lands independently and preserves the prior desktop experience.

### 9.1 Sequencing principle

D38b-impl-1 lands the foundation. At this point, the Explorer **looks identical at desktop and continues to be unusable at phone** — but the foundation is in place. This is intentional: it lets the foundation be reviewed in isolation without entangling shell + graph changes.

D38b-impl-2 lands shell responsiveness. Now the Explorer is **usable at tablet** and **partly usable at phone** (catalogue, records, settings work; graph mode is broken on phone). Tablets are the primary tablet-tier use case (bank reviewer with iPad) and shipping value at this point is high.

D38b-impl-3 lands graph responsiveness. Now the Explorer is **usable at all four tiers** with the documented quality contract (graph degrades gracefully on phone).

### 9.2 What does not change in any tranche

- The existing `body.sidebar-collapsed` and `body.inspector-collapsed` user-toggle classes — preserved.
- The existing `data-theme="light"` opt-in — preserved.
- The dark default theme — preserved.
- All existing CSS custom properties in tokens.css — preserved (additive only).
- The Cytoscape resize-handling architecture — preserved.
- The OpenAPI contract — unaffected (no spec changes).
- The Go backend — completely unaffected.

### 9.3 Reversibility

If a tranche causes unforeseen issues, it can be reverted by removing the new files and reverting the per-file rule migrations. Because the migration is additive, no data or contract is at risk.

## 10. Implementation tranche sequence

The proposal decomposes into three implementation tranches (with optional follow-ups):

| Tranche | Scope | Estimated effort | Visible value at desktop | Visible value at tablet | Visible value at phone |
|---|---|---|---|---|---|
| **D38b-impl-1 — Foundation** | Breakpoint tokens, fluid typography migration, hit-target token, four a11y media queries | 1 working session | None (visual byte-identical) | None (rules not yet applied) | None | 
| **D38b-impl-2 — Shell responsive** | Sidebar tier behaviour, inspector rail tier behaviour, header tier behaviour, hamburger + off-canvas drawer pattern | 2 working sessions | None at default; user-toggleable nav | iPad becomes usable | Catalogue/records/settings usable; graph still broken |
| **D38b-impl-3 — Graph responsive** | Viewport-tier signal, Cytoscape style adjustments, camera control sizing, phone-tier summary list | 2 working sessions | None | Graph touch-usable on iPad | Graph degrades gracefully; phone fully supported |
| **D38b-impl-4 — Touch & a11y polish** *(optional)* | Audit and fix per-component hit targets the foundation selector list missed; refine focus-visible coverage; add `prefers-contrast` overrides | 1 working session | A11y test pass | A11y test pass | A11y test pass |
| **D38b-impl-5 — Container queries** *(optional, deferred)* | Migrate specific components (records detail, drift workbench cards) to container queries | 1 working session per component | Improved component reuse at non-standard widths | Same | Same |

**Total core effort: ~5 working sessions across three tranches.**

## 11. Open questions / decisions deferred to implementation

1. **Exact phone-tier graph degradation behaviour.** Section 8.4 proposes a summary card list as the phone-tier alternative to graph manipulation. The exact information density of the summary list (which fields? which sort order? how is selection state visualised?) is an information architecture decision that belongs to D38b-impl-3's design phase.
2. **Inspector drawer triggering on phone.** Section 7.2 says the inspector drawer is triggered by canvas-edge tabs. On phone, when the canvas itself is reduced to a summary list, the trigger mechanism needs reconsideration — likely "tap a list item opens the inspector drawer" rather than canvas-edge tabs.
3. **Whether the existing hamburger CSS pattern exists elsewhere in MIDAS.** Grep returned 179 `drawer` matches — none for a hamburger pattern. The implementation tranche should confirm before designing.
4. **CSS organisation post-migration.** Currently the responsive rules are scattered (15 rules across 8 files). Post-migration there is a single `shell-responsive.css` plus per-feature responsive rules. Whether to migrate the existing 15 rules out of their host files into a feature-organised `*-responsive.css` per host file is a judgement call for impl-1 or impl-2.
5. **Whether to use `dvh` / `svh` viewport units for the shell.** iOS Safari's URL bar collapse changes `vh` mid-session; `dvh` (dynamic viewport height) is more accurate. Browser support is recent (2023+) — should be confirmed during impl-2.
6. **JS-side viewport detection vs CSS-only.** Section 8.2 proposes a JS module that mirrors the CSS viewport tier as a body attribute. An alternative is pure CSS using `@media` rules inside the Cytoscape style strings. The JS approach is more flexible (programmatic decisions based on tier) but adds a module; CSS-only is simpler but less flexible. Decide at impl-3.
7. **Whether to ship a phone-tier landing page change.** The current README + getting-started flow assumes desktop. A phone user landing on `/explorer` for the first time may benefit from a "this works best on desktop; tablet/phone has reduced functionality" hint. Could be part of impl-2 or a separate D38c tranche.

These are all deferred to implementation — listing them here is to ensure they are not forgotten, not to resolve them in this design.

## 12. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Migrating 15 existing breakpoint values causes visual regressions at intermediate viewports | Medium | Low | Implementation tranche includes a visual diff pass at the four breakpoint boundaries (480, 1024, 1440 px) plus 720 and 1100 px (old boundaries). |
| Fluid typography migration changes effective font-sizes for users with non-default browser zoom | Low | Low–medium | Body root at explicit 16px; component sizes computed as rem against 16px so default behaviour is byte-identical. Users with non-default zoom see proportional sizing — almost always desirable. |
| The hit-target selector list misses interactive elements, leaving some buttons sub-44px on touch | Medium | Medium | Implementation tranche includes an enumeration step against the actual rendered DOM; D38b-impl-4 catches misses. |
| Cytoscape style overrides at tablet/phone tier break existing graph rendering at desktop | Low | High | Tier signal is opt-in per stylesheet rule; desktop tier sees no rule applied. Cytoscape style changes are scoped to the tier-conditional class. |
| `prefers-reduced-motion` global rule suppresses motion the team wants to keep (e.g. micro-interactions critical to feedback) | Low | Low | The proposed rule uses `transition-duration: 0.01ms !important` rather than `transition: none` — preserves the transition existence (so JS that triggers on `transitionend` continues to fire) while making it instantaneous. Any rule the team wants to keep can be excepted with a more specific selector. |
| Off-canvas drawer pattern conflicts with the existing right-rail drawer terminology and codebase (179 `drawer` matches) | Medium | Low | Implementation tranche names the new pattern explicitly (`sidebar-drawer`, `inspector-drawer`) and avoids the unqualified `drawer` term. |
| Phone-tier graph degradation is seen as a missing feature rather than graceful degradation | Low | Medium | Document the design choice clearly in user-facing documentation; tranche D38b-impl-3 ships a brief help page entry explaining the rationale. |

## 13. Out of scope (firm)

- Anything that changes the Go backend.
- Anything that changes the OpenAPI contract.
- Anything that changes the Postgres schema.
- Anything that changes the narrative documentation set's structure (per-tranche doc updates within existing files are fine).
- The optional D38b-impl-4 (touch & a11y polish) — recommended but not load-bearing for the proposal.
- The optional D38b-impl-5 (container queries) — deferred until a specific component justifies the investment.
- PWA, offline support, install banners, push notifications.
- A native mobile app.
- A rebrand or visual identity refresh.
- Anything affecting the userguide (the MkDocs Material default is already responsive; no Explorer-tranche work needed for the userguide).

## 14. Decision required

To proceed with implementation, the maintainer team confirms:

1. The four-tier breakpoint system (480 / 1024 / 1440) is acceptable as proposed.
2. The phone-tier graceful-degradation model (summary list replaces graph manipulation) is acceptable.
3. The migration of 15 existing breakpoint literals to the four-tier system is acceptable.
4. The estimated 5 working sessions across three tranches fits the roadmap.
5. The optional impl-4 (touch & a11y polish) is desirable.
6. The optional impl-5 (container queries) is deferred without commitment.

Once confirmed, D38b-impl-1 (foundation) can be scheduled as the next responsive-track tranche.

## 15. References

- D38a-responsive-design-audit-assess — the predecessor audit; verdict PARTIAL, HIGH confidence.
- [internal/httpapi/explorer/assets/css/tokens.css](../../internal/httpapi/explorer/assets/css/tokens.css) — existing token foundation.
- [internal/httpapi/explorer/assets/css/shell.css](../../internal/httpapi/explorer/assets/css/shell.css) — shell layout.
- [internal/httpapi/explorer/assets/css/services.css:480](../../internal/httpapi/explorer/assets/css/services.css) — the load-bearing existing responsive rule.
- [internal/httpapi/explorer/assets/js/graph/graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js) — existing Cytoscape resize handling.
- WCAG 2.5.5 Target Size (Enhanced) — 44 × 44 px minimum for touch targets.
- Apple Human Interface Guidelines — 44 × 44 pt minimum touch target.
- Material Design — 48 × 48 dp minimum touch target.
- MDN: `prefers-reduced-motion`, `prefers-color-scheme`, `prefers-contrast`, `pointer: coarse`.
