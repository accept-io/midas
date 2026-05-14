# D32h-fix-2d-parity — Authority Workbench Letterbox Parity Analysis

**Tranche:** D32h-fix-2d-parity (analysis only — no production code modified).
**Working constraint:** local-only. Read-only inspection.
**Prior:** [D32h-fix-2d](../implementation/D32h-fix-2d-authority-bottom-workbench-letterbox.md) made the Authority workbench *visible* by deleting an `!important` CSS override. The operator's follow-up screenshot shows that **even when visible, the Authority workbench differs materially from Context's Drift Analytics letterbox**. This document documents the divergence so a follow-up tranche can converge them.

---

## 1. Executive Summary

**There are two parallel letterbox implementations.** They occupy the same DOM column under `.governance-map-workbench` and respond to the same `body[data-graph-lens]` switch, but everything else — the close-arrow behaviour, the collapsed-state height model, the transition target, the header chrome, the colour-token palette — is independently authored. The bottom letterbox is **not** a reusable shell slot. It is two siblings the active lens swaps between.

**Three operator-reported defects all originate from this divergence:**

| Operator report | Root cause |
| --- | --- |
| "Authority workbench has no close/collapse control" | The control exists in DOM ([index.html:618-624](../../internal/httpapi/explorer/index.html#L618-L624)) but clicking it has no visible effect because the CSS only animates `max-height` (280 ↔ 320), not `height`. Content fills from `min-height: 36px` up to whichever cap is active — both caps are >> header height, so the workbench paints identically whether `.is-expanded` is on or off. The glyph also never updates (Context flips ▲↔▼; Authority leaves it ▲ permanently). |
| "In Workbench Focus Mode, the letterbox should default to closed when Authority lens is active. Currently defaults open." | **Neither letterbox listens for Focus Mode.** Both initialise `_expanded = false`. The reason Context *appears* closed-by-default in Focus Mode is that its collapsed state CSS `height: 36px` actually clips to header-only. Authority's collapsed-state CSS does nothing — `min-height: 36px; max-height: 280px` lets content drive the displayed height, so the workbench paints near-full even when `_expanded === false`. The operator perceives this as "defaults open." |
| "Underlying CSS appears to differ" | Confirmed: different height model, different transition target, different padding, different token palette. Context uses Material Design tokens (`--surface-container-low`, `--outline-variant`, `--slate-300`, `--font-display`); Authority uses `var(--panel-bg, #0f1115)`-style hard-coded RGB fallbacks. |

**Architectural verdict:** Two parallel implementations. The convergence work is real but small.

**Recommendation: option (a) — port Authority's behaviour to match Context's collapsed-state CSS contract.** This fixes all three reported defects (close-arrow becomes effective; Focus-Mode default-collapsed becomes real; visual divergence narrows substantially) without extracting a shared module. Option (b) — extract a shared letterbox-chrome module — is the architecturally cleaner answer but is **significantly larger work** and is not justified by the immediate operator pain. Option (c) — patch Authority piecemeal without aligning the height model — would leave the close-arrow defect un-fixed.

**Scoped fix-tranche proposal:** **`D32h-fix-2d-converge`** — port the Authority workbench's CSS to match Context's height-driven collapse model (concrete CSS rewrite scoped to ~12 lines), add a glyph-update branch to the JS toggle handler, and add a parity test that pins the height model. ~30 lines of production change, 3-4 tests. No DOM changes. No module API changes. No Context changes. Browser verification required for collapse animation behaviour.

---

## 2. Context Implementation (Drift Analytics letterbox)

### 2.1 Close / collapse control

| Aspect | Value |
| --- | --- |
| Toggle DOM | `<button id="gmap-evidence-tray-toggle" class="gmap-evidence-tray-toggle" aria-expanded="false" aria-controls="gmap-evidence-tray" aria-label="Expand Drift Analytics tray" title="Expand Drift Analytics tray"><span aria-hidden="true">▲</span></button>` at [index.html:568-570](../../internal/httpapi/explorer/index.html#L568-L570) |
| Click handler | [context-evidence-tray.js:1048-1055](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L1048-L1055) — `toggle.addEventListener('click', function () { gmapEvidenceTrayExpanded = !gmapEvidenceTrayExpanded; applyGmapEvidenceTrayState(); });` |
| State model | Module-private `let gmapEvidenceTrayExpanded = false;` at [context-evidence-tray.js:101](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L101) |
| State application | `applyGmapEvidenceTrayState()` at [context-evidence-tray.js:1007-1044](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L1007-L1044) toggles `.is-expanded`, updates ARIA, **swaps the glyph (`▼` when expanded, `▲` when collapsed)**, and schedules a `fitGmapToBounds()` re-fit on a 200ms timeout so the graph repositions to the new available canvas-scroll height after the CSS transition completes. |

### 2.2 State persistence

- **Across lens switches:** module-local. No persistence; the tray state survives only as long as the module instance does. Since the module lives the whole session, in practice it persists within a single session.
- **Across page reloads:** **No persistence.** No localStorage / sessionStorage write found in the module.

### 2.3 Focus Mode interaction

**No automatic Focus-Mode → tray-close behaviour exists in code.** The Context tray's `gmapEvidenceTrayExpanded` is `false` at module init (line 101). Focus Mode is a `body.gmap-focus-mode` class managed by `applyGmapFocusMode()` at [index.html:3014-3076](../../internal/httpapi/explorer/index.html#L3014-L3076), which does **not** touch the evidence tray.

The reason Context *appears* closed-by-default whenever the operator enters Focus Mode is downstream: the tray starts collapsed (`gmapEvidenceTrayExpanded = false`) and the CSS height-model honours that state (`.gmap-evidence-tray { height: 36px; }` shows only the 36px header bar). It's a coincidence of initialisation, not an explicit Focus-Mode coupling.

### 2.4 CSS rule set ([governance-map.css:1246-1370](../../internal/httpapi/explorer/assets/css/governance-map.css#L1246-L1370))

```css
.gmap-evidence-tray {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--surface-container-low);
  border-top: 1px solid var(--outline-variant);
  height: 36px;                                     /* ← collapsed default */
  overflow: hidden;
  transition: height 0.18s ease-out;                /* ← animates height */
}
.gmap-evidence-tray.is-expanded {
  height: 320px;                                    /* ← expanded fixed */
}
.gmap-evidence-tray-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 14px;
  height: 36px;                                     /* ← matches collapsed parent */
  flex-shrink: 0;
  background: var(--surface-container-lowest);
  border-bottom: 1px solid var(--outline-variant);
  font-family: var(--font-display);
}
/* Toggle button styling — uses --surface-container-low, --on-surface, --primary, --slate-400 */
.gmap-evidence-tray-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 22px;
  background: var(--surface-container-low);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 4px;
  color: var(--slate-400);
  ...
}
```

**Key load-bearing properties:**
- `height: 36px` collapsed → only the header is visible.
- `overflow: hidden` → body content is clipped when collapsed.
- `transition: height 0.18s ease-out` → smooth animation when toggling.
- Material Design colour tokens throughout.

---

## 3. Authority Implementation (Authority Workbench letterbox)

### 3.1 Close / collapse control

| Aspect | Value |
| --- | --- |
| Toggle DOM | `<button type="button" id="gmap-authority-workbench-toggle" class="gmap-authority-workbench-toggle" aria-expanded="false" aria-controls="gmap-authority-workbench" aria-label="Expand Authority Workbench" title="Expand Authority Workbench"><span aria-hidden="true">▲</span></button>` at [index.html:618-624](../../internal/httpapi/explorer/index.html#L618-L624) |
| Click handler | [authority-graph-workbench.js:564-567](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L564-L567) — `if (toggle) { toggle.addEventListener('click', function () { _setExpanded(!_expanded); }); }` |
| State model | Module-private `var _expanded = false;` at [authority-graph-workbench.js:84](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L84) |
| State application | `_setExpanded(expanded)` at [authority-graph-workbench.js:87-98](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L87-L98) toggles `.is-expanded` and updates `aria-expanded` / `aria-label` / `title`. **Does NOT swap the glyph.** **Does NOT schedule a re-fit.** |

### 3.2 State persistence

- **Across lens switches:** module-local. Same as Context.
- **Across page reloads:** **No persistence.** Same as Context.

### 3.3 Focus Mode interaction

**No automatic Focus-Mode → letterbox-close behaviour.** Same as Context. The Authority workbench's `_expanded` is `false` at module init. Focus Mode does not touch it.

However: Authority's CSS does **not** honour the `_expanded === false` state visually — see §3.4. Result: the operator's perception that "Authority defaults open in Focus Mode" is a CSS defect, not a Focus-Mode coupling gap.

### 3.4 CSS rule set ([authority-graph.css:796-…](../../internal/httpapi/explorer/assets/css/authority-graph.css#L796))

```css
.gmap-authority-workbench {
  flex-direction: column;
  background: var(--panel-bg, #0f1115);             /* ← hard-coded fallback */
  border-top: 1px solid var(--border-color, rgba(255,255,255,0.08));
  color: var(--text-primary, #f3f4f6);
  min-height: 36px;                                 /* ← floor, not collapsed-default */
  max-height: 280px;                                /* ← ceiling when not expanded */
  overflow: hidden;
  transition: max-height 200ms ease;                /* ← animates max-height, not height */
}
.gmap-authority-workbench.is-expanded {
  max-height: 320px;                                /* ← only nudges ceiling */
}
.gmap-authority-workbench-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;                                /* ← no explicit height */
  border-bottom: 1px solid var(--border-color, rgba(255,255,255,0.06));
  background: var(--panel-bg-strong, rgba(255,255,255,0.02));
}
/* Toggle button styling — uses hard-coded RGB fallbacks, NOT Material tokens */
.gmap-authority-workbench-toggle {
  background: transparent;
  border: 1px solid var(--border-color, rgba(255,255,255,0.16));
  color: var(--text-primary, #f3f4f6);
  width: 28px;
  height: 28px;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
```

**Key DIVERGENT properties:**
- `min-height: 36px; max-height: 280px` instead of `height: 36px` / `height: 320px`. Content drives actual height; the workbench expands to its natural content size up to the cap. **The collapse state is not visually distinguishable from the expand state**, because both states allow content to fill from 36px upward.
- `transition: max-height 200ms ease` — animates the wrong property. `max-height` only constrains; toggling it from 280 → 320 produces a 40px visual nudge at best.
- `padding: 8px 16px` on the header without an explicit `height: 36px`. The header height is content-driven, not fixed.
- Colour tokens: `var(--panel-bg, #0f1115)` / `var(--border-color, …)` / `var(--text-primary, …)` / `var(--text-muted, …)` — these `--panel-bg` etc. tokens are **not defined elsewhere in the explorer CSS** (only the hard-coded fallbacks fire). Context uses `var(--surface-container-low)` etc., which ARE defined in `tokens.css`.

---

## 4. Parity Gap Table

| Aspect | Context | Authority | Divergent? | Severity |
| --- | --- | --- | --- | --- |
| Toggle button DOM | `<button id="gmap-evidence-tray-toggle">` with `▲` glyph | `<button id="gmap-authority-workbench-toggle">` with `▲` glyph | **Same shape, different ids** | Low (intentional namespacing) |
| Toggle click handler | Flips bool, calls applyState | Flips bool, calls _setExpanded | Functionally equivalent | Low |
| Toggle glyph update on state change | **`▲ ↔ ▼`** at [context-evidence-tray.js:1016-1017](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L1016-L1017) | **No glyph update** ([authority-graph-workbench.js:87-98](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L87-L98) only updates aria + title) | **DIVERGENT** | **High** — operator perceives "no close control" because the affordance is visually static |
| Re-fit on expand/collapse | `setTimeout(fitGmapToBounds, 200)` at [L1039-1043](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L1039-L1043) | **Not scheduled** | **DIVERGENT** | Medium — graph doesn't reposition when Authority workbench expands |
| State model | `let gmapEvidenceTrayExpanded` module-private | `var _expanded` module-private | Equivalent | Low |
| Open/closed default (non-Focus mode) | `false` (closed) | `false` (closed) | Equivalent at code level | Low — but see "Default visual outcome" row |
| Open/closed default (Focus mode) | `false` (closed) — same as non-Focus | `false` (closed) — same as non-Focus | Equivalent at code level. **Neither listens for Focus Mode**. | Low at code level, but operator perceives Authority as open by default — see "Default visual outcome" row |
| **Default visual outcome at runtime** | **Collapsed = 36px header bar only** ([governance-map.css:1252](../../internal/httpapi/explorer/assets/css/governance-map.css#L1252)) | **Collapsed = content-driven height up to 280px** ([authority-graph.css:801-802](../../internal/httpapi/explorer/assets/css/authority-graph.css#L801-L802)) | **DIVERGENT** | **High** — root cause of "Authority defaults open" complaint |
| State persistence across lens switches | Module-local | Module-local | Equivalent | Low |
| State persistence across page reloads | None | None | Equivalent | Low |
| Header layout | `display: flex; gap: 12px; padding: 8px 14px; height: 36px; flex-shrink: 0;` | `display: flex; gap: 12px; padding: 8px 16px;` (no explicit height) | **DIVERGENT** | Medium — Authority header height is content-driven |
| Tab styling | `.gmap-evidence-tray-tab` (governance-map.css) | `.gmap-authority-workbench-tab` (authority-graph.css) — different colour tokens | **DIVERGENT** | Medium — visual mismatch |
| Body styling | `.gmap-evidence-tray-body { flex: 1 1 auto; min-height: 0; }` | `.gmap-authority-workbench-body { display: flex; flex-direction: column; overflow: hidden; }` | **DIVERGENT** | Medium — different overflow / min-height contracts |
| Collapsed height | `36px` | Effectively content-driven (min 36, max 280) | **DIVERGENT** | **High** |
| Expanded height | `320px` (fixed) | Effectively content-driven (min 36, max 320) | **DIVERGENT** | Medium |
| Transition target | `height 0.18s ease-out` | `max-height 200ms ease` | **DIVERGENT** | Medium — Authority animates the wrong property |
| Colour-token palette | `--surface-container-low`, `--outline-variant`, `--slate-300/400/500`, `--on-surface`, `--primary`, `--font-display` (Material Design tokens defined in `tokens.css`) | `var(--panel-bg, #0f1115)`, `var(--border-color, …)`, `var(--text-primary, …)`, `var(--text-muted, …)`, `var(--accent-emphasis, …)` — **these tokens are not defined elsewhere in the explorer CSS; only the hard-coded RGB fallbacks fire** | **DIVERGENT** | **High** — Authority workbench paints in pure hard-coded colours; theme switching (light/dark) cannot affect it |

---

## 5. Architectural Finding

**The bottom letterbox is not a reusable shell slot.** There are two parallel implementations:

- `#gmap-evidence-tray` (Context) — owned by `context-evidence-tray.js`, styled in `governance-map.css`. Predates Authority by many tranches (D24g / D25b).
- `#gmap-authority-workbench` (Authority) — owned by `authority-graph-workbench.js`, styled in `authority-graph.css`. Added by D32h-fix-1.

They share:
- The same parent column `.governance-map-workbench`.
- The same lens-routing mechanism (`body[data-graph-lens]` CSS rules).
- A coincidence of structure (both have a header row with a toggle button, a tabs row, a body panel).

They diverge on:
- Every styling detail (height model, transition target, padding, tokens).
- Behaviour details (Context updates glyph and schedules re-fit; Authority does neither).
- Colour palette (Context uses Material Design tokens; Authority uses undefined tokens with hard-coded fallbacks).

D32h-fix-1's deliverable described the workbench as "a sibling tray under the same parent column so the canvas reserves the same vertical space regardless of lens — switching lenses does not reflow the canvas." That framing was structurally accurate but **understated the divergence**: the Authority module re-authored every styling and behaviour decision rather than importing or reusing Context's. The result is two siblings that look unrelated up close.

**Reusable-shell-slot claim status: false.** This is two implementations swapped by lens routing, not a slot that lenses plug content into.

---

## 6. Convergence Approach Recommendation

Three options ranked by risk × value:

### (a) Patch Authority's CSS to match Context's height-driven collapse — **RECOMMENDED**

- **What:** Rewrite the ~12 lines of Authority workbench CSS to use `height: 36px` collapsed / `height: 320px` expanded with `transition: height 0.18s ease-out`. Switch token references from `var(--panel-bg, …)` etc. to the Material Design tokens Context uses. Add a glyph-update branch to `_setExpanded` in the JS module.
- **What it fixes:** All three operator-reported defects in one pass. Close-arrow becomes effective (collapsed = 36px). Focus-Mode default-closed becomes real (because the underlying default-collapsed state now actually collapses). Visual divergence narrows substantially.
- **What it does not fix:** The architectural duplication — there are still two CSS rule sets, two modules, two states. But the visible product becomes indistinguishable between lenses up to content.
- **Risk:** Low. Scope is bounded to Authority CSS + one JS function. No Context changes. No DOM changes. No module API changes. Existing D32hFix1 / D32hFix2b / D32hFix2c / D32hFix2d source-string tests should all still pass.
- **Browser verification:** required for the height-transition animation behaviour. Add a Tier-1 source-string test pinning the new height-based CSS rules; defer animation feel to D32h-fix-2h's snapshot pass.

### (b) Extract a shared letterbox-chrome module

- **What:** Create `internal/httpapi/explorer/assets/js/graph/graph-letterbox.js` plus a shared `graph-letterbox` CSS class set. Both Context and Authority lens trays consume the same chrome (toggle button + glyph update + transition + collapsed/expanded heights); only the body content differs per lens.
- **What it fixes:** Same as (a), plus the architectural duplication.
- **Risk:** **Significantly higher.** Context's `applyGmapEvidenceTrayState` does Context-specific work alongside the chrome (renders the drift panel, loads activity data, schedules `fitGmapToBounds`). Disentangling chrome from lens-specific behaviour requires either an event/callback contract or a render-into-mount split that doesn't exist today. Multiple D32h-fix-1 / D32h-fix-2b / D32h-fix-2c / D24g / D25b tests pin the current Context-tray module exports and would need re-aligning if Context lost its private state.
- **When to do this:** If a third lens appears, or if the convergence-by-CSS approach proves brittle in practice. Not the right scope for the immediate operator pain.

### (c) Patch Authority piecemeal without aligning the height model

- **What:** Add the glyph update. Maybe add a "close" button. Don't touch the CSS height model.
- **What it fixes:** Operator-reported defect #1 (close-arrow) partially — the glyph would update, but clicking it would still have no visible effect because `_expanded === false` doesn't actually collapse the workbench visually.
- **Why rejected:** Leaves the load-bearing bug (height model mismatch) unresolved. The operator's "defaults open in Focus Mode" report cannot be fixed without aligning the height model.

**Recommendation:** **(a) Patch Authority's CSS.** Smallest scope that resolves all three operator-reported defects. Defers (b)'s architectural cleanup to a future tranche when a third lens or additional reuse pressure forces the issue.

---

## 7. Scoped Fix-Tranche Proposal — `D32h-fix-2d-converge`

### Tranche goal

Port the Authority workbench's CSS and toggle behaviour to match Context's letterbox contract, so the close-arrow actually collapses the workbench and the default-collapsed state is visually honoured. Resolves the three operator-reported defects.

### Scope

| File | Change |
| --- | --- |
| [authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) | Rewrite `.gmap-authority-workbench` rule block: switch from `min-height: 36px; max-height: 280px` to `height: 36px`; switch `.is-expanded` from `max-height: 320px` to `height: 320px`; switch `transition: max-height …` to `transition: height …`. Add explicit `height: 36px; flex-shrink: 0;` to `.gmap-authority-workbench-header`. Migrate colour-token references from `var(--panel-bg, …)` etc. to Material Design tokens (`--surface-container-low`, `--outline-variant`, `--slate-300/400`, `--on-surface`, `--primary`, `--font-display`) so theme switching works. |
| [authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js) | Add glyph-update branch to `_setExpanded`: `var glyph = btn && btn.querySelector('span'); if (glyph) glyph.textContent = _expanded ? '▼' : '▲';`. Optionally: schedule a `fitGmapToBounds()` (or the lens-aware equivalent) on a 200ms timeout after the transition completes, mirroring Context's behaviour. |
| [internal/httpapi/explorer_d32h_fix2d_converge_test.go](../../internal/httpapi) | **New file.** Tests below. |

### Out of scope

- Extracting a shared letterbox-chrome module (option b).
- Adding Focus-Mode → tray-close coupling. Neither lens does this today; pairs with a future Focus-Mode tranche if needed.
- Persisting expand/collapse state across page reloads.
- Touching Context implementation files.
- Changing the Authority DOM structure (the `<section id="gmap-authority-workbench">` shape stays).

### Tests required

1. **`TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesHeightCollapseModel`** — pins that `.gmap-authority-workbench { ...height: 36px... }` appears in the served CSS and `.gmap-authority-workbench.is-expanded { ...height: 320px... }` appears, ban the previous `min-height: 36px; max-height: 280px` form.
2. **`TestExplorer_D32hFix2dConverge_AuthorityWorkbenchTransitionsHeight`** — pins `transition: height ...` on the workbench root and bans `transition: max-height ...` on the same selector.
3. **`TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHeaderHasFixedHeight`** — pins explicit `height: 36px` on the header.
4. **`TestExplorer_D32hFix2dConverge_AuthorityWorkbenchToggleUpdatesGlyph`** — pins the glyph swap branch in `_setExpanded`.
5. **`TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesSharedTokens`** — pins that `--surface-container-low` / `--outline-variant` / `--slate-300` (or equivalent shared tokens) appear in the Authority workbench's CSS rules, banning `var(--panel-bg, ...)`.

Plus regression guards: all D32hFix1, D32hFix2b, D32hFix2c, D32hFix2d tests must remain green.

### Browser verification required

Yes — the height-transition animation feel cannot be confirmed by source-string tests. Reader's smoke check after the tranche:

1. Load Authority lens. Workbench should show only the 36px header bar (not the previous full-height "open by default" appearance).
2. Click the toggle. Workbench expands to 320px with a ~0.18s animation; glyph flips ▲ → ▼.
3. Click again. Workbench collapses back to 36px; glyph flips ▼ → ▲.
4. Switch to Context lens. Drift Analytics tray still works identically (Context untouched).
5. Switch back to Authority. State persists within the session (same as Context).

Formal verification via the snapshot harness if desired.

### Acceptance criteria

- All three operator-reported defects from the parity report are resolved at code level.
- All D32hFix1 / D32hFix2b / D32hFix2c / D32hFix2d / D32g / D32f source-string pins remain green.
- Full `./test.sh all` green.
- No Context implementation changes.
- No DOM changes (the `<section>` shape from D32h-fix-1 is byte-identical).
- No backend / schema / OpenAPI / seed / runtime changes.
- No GitHub operations.

### Recommended priority

**Next**, ahead of `D32h-fix-2e` (shared-node badges) and `D32h-fix-2f` (browser-verified refinement). The three operator-reported defects are blocking productisation of the Authority workbench — fix them before adding more features. Cleanup tranches (`-2g`, `-2h`) remain last per the roadmap.

---

## 8. Constraints Confirmation

- No production code modified by this tranche.
- No DOM, module, CSS, test, backend, schema, OpenAPI, seed, runtime, deployment, or GitHub changes.
- Only file written: this analysis report at [docs/analysis/D32h-fix-2d-parity-letterbox-divergence.md](./D32h-fix-2d-parity-letterbox-divergence.md).
- Read-only inspection mode honoured throughout.

**End of D32h-fix-2d-parity.**
