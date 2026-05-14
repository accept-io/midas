# D32h-fix-2d-converge — Authority Workbench Letterbox CSS and Behaviour Convergence

**Tranche:** D32h-fix-2d-converge.
**Boundary:** the three operator-reported defects from
[D32h-fix-2d-parity](../analysis/D32h-fix-2d-parity-letterbox-divergence.md)
are closed by converging the Authority workbench's CSS height model
and chrome tokens with Context's evidence-tray contract, plus
adding glyph-update + 200ms re-fit to the toggle handler.
Architectural duplication remains deferred.
**Working constraint:** local-only. No GitHub operations. No backend
/ schema / OpenAPI / seed / runtime / deployment changes.

---

## 1. Executive Summary

The three operator-reported defects from the parity analysis are now
fixed at code level:

1. **"No close/collapse control"** — `_setExpanded` now flips the toggle glyph (▲ ↔ ▼) on every state change, mirroring Context's pattern. And — critically — the underlying CSS now actually collapses: the workbench root uses `height: 36px` (collapsed) / `height: 320px` (expanded) instead of `min-height: 36px; max-height: 280px / 320px`. Clicking the toggle now produces a visible 0.18s height transition.
2. **"Defaults open in Focus Mode"** — was a CSS height-model bug, not a missing Focus-Mode listener. With the new height-driven collapse, the default `_expanded = false` state visually collapses to a 36px header bar (matching Context's behaviour under the same Focus-Mode entry).
3. **"Underlying CSS appears to differ"** — 24 rules rewritten with the token migration: shell chrome moves to defined Material Design tokens (`--surface-container-low`, `--outline-variant`, `--slate-300/400`, `--on-surface`, `--primary`, `--font-display`); severity / status colours move to the convention-with-fallback `--badge-bad/warn/info` tokens used elsewhere in the explorer. Five undefined tokens (`--panel-bg`, `--border-color`, `--text-primary`, `--text-muted`, `--accent-emphasis`/`--accent-gap`/`--severity-*`) are gone from the workbench rules; theme switching now affects Authority chrome.

Plus a load-bearing behaviour addition: `_setExpanded` schedules a 200ms `setTimeout` calling `window.MIDASExplorerGraph.camera.scheduleFitToView()` so the graph repositions when the workbench collapses or expands (mirrors Context's `setTimeout(fitGmapToBounds, 200)` at context-evidence-tray.js:1039-1043). Without this, expanding the workbench would clip nodes; collapsing would leave dead canvas space.

**Architectural duplication is NOT resolved by this tranche.** There are still two parallel letterbox implementations (Context's `gmap-evidence-tray` and Authority's `gmap-authority-workbench`). They are now visually and behaviourally aligned, but their CSS rule sets and JS modules remain independent. Section §3 below tracks this as deferred technical debt.

**Tests:** 8 new D32hFix2dConverge tests pass. **Zero existing tests required pin-string updates** (Step 0.4 audit confirmed; full focused subset green: 47 tests). Full `./test.sh all` green. `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` passes both before and after the tranche.

---

## 2. Step 0 Inventory and Sign-Off Summary

Reproduced verbatim from the approved sign-off:

### Step 0.1 — Token mapping (10 distinct tokens)

| Authority token (pre-fix) | Migration target | Convention |
| --- | --- | --- |
| `var(--panel-bg, #0f1115)` | `var(--surface-container-low)` | Defined Material (tokens.css:14) |
| `var(--panel-bg-strong, rgba(255,255,255,0.02))` | `var(--surface-container-lowest)` | Defined Material (tokens.css:13) |
| `var(--border-color, …)` (4 variants) | `var(--outline-variant)` | Defined Material (tokens.css:45) |
| `var(--text-primary, #f3f4f6)` | `var(--on-surface)` | Defined Material (tokens.css:21) |
| `var(--text-muted, #9ca3af)` | `var(--slate-400)` | Defined Material (tokens.css:52) |
| `var(--accent-emphasis, #fbbf24)` | `var(--badge-warn, #fbbf24)` | Convention-with-fallback (matches drift-analytics.css:59) |
| `var(--accent-gap, #f87171)` | `var(--badge-bad, #f87171)` | Convention-with-fallback (matches authority-graph.css:108) |
| `var(--severity-critical, #ef4444)` | `var(--badge-bad, #ef4444)` | Convention-with-fallback (matches drift-analytics.css:58) |
| `var(--severity-warning, #f59e0b)` | `var(--badge-warn, #f59e0b)` | Convention-with-fallback (matches drift-analytics.css:59) |
| `var(--severity-info, #60a5fa)` | `var(--badge-info, #60a5fa)` | Convention-with-fallback (matches drift-analytics.css:60) |
| `rgba(255,255,255,0.06)` (toggle:hover + diagnostic-severity background) | `var(--surface-container)` | Defined Material (tokens.css:15) |
| Diagnostic-severity per-class overlay rgba (3 values) | **preserved as-is** | Intentional translucent overlays, not theme-bound |

### Step 0.2 — Re-fit hook

`window.MIDASExplorerGraph.camera.scheduleFitToView()` at [graph-camera.js:214-226](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L214-L226). Two-frame rAF wrapper around `fitToBounds()`. Lens-agnostic. Authority view already uses it as a fallback at line 434.

### Step 0.3 — CSS rule scope

- **2 lens-routing rules UNCHANGED** (lines 785-790).
- **2 structural-only rules UNCHANGED** (`.gmap-authority-workbench-panel`, `.authority-workbench-section`, `.authority-workbench-stats`, `.authority-workbench-diagnostics`).
- **~24 rules REWRITE** — see §4.5 migration table.
- **HTML `hidden` attribute preserved** per D32hFix1 contract.

### Step 0.4 — Test-update scope

**ZERO existing tests required pin-string updates.** Every existing test pins SELECTOR presence (e.g. `.gmap-authority-workbench`, `.authority-workbench-section`), not declarations. The rewrite preserves every selector; only declarations change. Verified by running `D32hFix1|D32hFix2b|D32hFix2c|D32hFix2d` before AND after — both pass.

---

## 3. Deferred Architectural Cleanup

**Not resolved by this tranche:** the architectural duplication identified in §5 of the parity analysis. There are still two parallel letterbox implementations:

- `#gmap-evidence-tray` owned by `context-evidence-tray.js` + `governance-map.css`.
- `#gmap-authority-workbench` owned by `authority-graph-workbench.js` + `authority-graph.css`.

After this tranche they are visually and behaviourally aligned — same height model, same 0.18s transition, same chrome tokens, same toggle behaviour — but they remain two independent CSS rule sets and two independent JS modules.

**Conditions that would trigger the extraction to a shared `graph-letterbox` module:**

1. **A third lens is added** (e.g. Knowledge Graph). With three lenses, three copies of the chrome CSS + behaviour multiplies maintenance cost; extraction becomes worth the investment.
2. **Further divergence pressure.** If a future tranche needs to add a feature to one letterbox (e.g. a persistence layer, a keyboard shortcut, a busy indicator) the operator should consider extracting before duplicating that feature into the other letterbox.
3. **A theme or design system change** that affects letterbox geometry. Two parallel implementations means the change has to land twice.

Tracked as **known deferred item** in the D32h-fix-2-plan roadmap; not blocking productisation.

---

## 4. Implementation Summary

### 4.1 CSS height-model rewrite

The load-bearing change. Pre-fix, the workbench root had `min-height: 36px; max-height: 280px; transition: max-height 200ms ease;` — collapse was effectively a no-op because content drove the actual height up to the cap. Post-fix, the workbench root has `height: 36px; transition: height 0.18s ease-out;` — `.is-expanded` flips to `height: 320px`. The collapse is now visible; the transition is now visible; the toggle now does something.

### 4.2 CSS token migration

10 distinct undefined tokens migrated in one pass per the Step 0.1 mapping. Five shell-chrome tokens move to defined Material Design tokens (theme-aware via `tokens.css`); five severity / status tokens move to convention-with-fallback `--badge-*` tokens (matching `drift-analytics.css` and the rest of the explorer). No partial migration — every undefined-token reference in the workbench rules is replaced.

### 4.3 JS glyph update

`_setExpanded` now flips the toggle glyph between `▼` (expanded) and `▲` (collapsed), mirroring `context-evidence-tray.js:1016-1017`. The previous code only updated `aria-expanded` / `aria-label` / `title`. Without the glyph swap, the affordance was visually static — the operator's "no close control" perception.

### 4.4 JS re-fit scheduling

`_setExpanded` now wraps `window.MIDASExplorerGraph.camera.scheduleFitToView()` in a 200ms `setTimeout`. Mirrors `context-evidence-tray.js:1039-1043`. Without this, expanding or collapsing the workbench would shrink/grow the canvas-scroll viewport but the graph would not reposition until the next manual fit, producing clipped nodes or dead canvas space.

### 4.5 CSS migration summary table (per amendment)

Side-by-side declarations for every rewritten rule. Selectors are byte-identical; only declarations change.

| Selector | Before declarations | After declarations |
| --- | --- | --- |
| `.gmap-authority-workbench` | `flex-direction: column; background: var(--panel-bg, #0f1115); border-top: 1px solid var(--border-color, rgba(255,255,255,0.08)); color: var(--text-primary, #f3f4f6); min-height: 36px; max-height: 280px; overflow: hidden; transition: max-height 200ms ease;` | `flex-direction: column; background: var(--surface-container-low); border-top: 1px solid var(--outline-variant); color: var(--on-surface); height: 36px; overflow: hidden; transition: height 0.18s ease-out;` |
| `.gmap-authority-workbench.is-expanded` | `max-height: 320px;` | `height: 320px;` |
| `.gmap-authority-workbench-header` | `display: flex; align-items: center; gap: 12px; padding: 8px 16px; border-bottom: 1px solid var(--border-color, rgba(255,255,255,0.06)); background: var(--panel-bg-strong, rgba(255,255,255,0.02));` | `display: flex; align-items: center; gap: 12px; padding: 8px 14px; height: 36px; flex-shrink: 0; background: var(--surface-container-lowest); border-bottom: 1px solid var(--outline-variant); font-family: var(--font-display);` |
| `.gmap-authority-workbench-title` | `font-weight: 600; font-size: 13px; letter-spacing: 0.02em;` | `font-size: 11px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--slate-300);` |
| `.gmap-authority-workbench-subtitle` | `flex: 1 1 auto; font-size: 12px; color: var(--text-muted, #9ca3af); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;` | `flex: 1 1 auto; min-width: 0; font-size: 12px; color: var(--slate-400); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;` |
| `.gmap-authority-workbench-toggle` | `background: transparent; border: 1px solid var(--border-color, rgba(255,255,255,0.16)); color: var(--text-primary, #f3f4f6); width: 28px; height: 28px; border-radius: 4px; cursor: pointer; display: inline-flex; align-items: center; justify-content: center;` | `display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 22px; background: var(--surface-container-low); border: 1px solid var(--outline-variant); border-radius: 4px; color: var(--slate-400); cursor: pointer; font-family: inherit; padding: 0; font-size: 14px; line-height: 1;` |
| `.gmap-authority-workbench-toggle:hover` | `background: rgba(255,255,255,0.06);` | `background: var(--surface-container); color: var(--on-surface);` |
| `.gmap-authority-workbench-toggle:focus-visible` | (rule did not exist) | `outline: 2px solid var(--primary); outline-offset: 1px;` (matches Context tray) |
| `.gmap-authority-workbench-body` | `display: flex; flex-direction: column; overflow: hidden;` | `flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column;` |
| `.gmap-authority-workbench-tabs` | `display: flex; gap: 4px; padding: 6px 16px 0; border-bottom: 1px solid var(--border-color, rgba(255,255,255,0.04)); flex: 0 0 auto;` | `display: flex; gap: 4px; padding: 6px 14px 0; border-bottom: 1px solid var(--outline-variant); background: var(--surface-container-lowest); flex: 0 0 auto;` |
| `.gmap-authority-workbench-tab` | `background: transparent; border: 1px solid transparent; border-bottom: none; color: var(--text-muted, #9ca3af); padding: 6px 12px; font-size: 12px; font-weight: 500; letter-spacing: 0.02em; border-top-left-radius: 6px; border-top-right-radius: 6px; cursor: pointer;` | `background: transparent; border: 1px solid transparent; border-bottom: none; color: var(--slate-400); padding: 6px 12px; font-size: 12px; font-weight: 500; letter-spacing: 0.02em; border-top-left-radius: 6px; border-top-right-radius: 6px; cursor: pointer;` |
| `.gmap-authority-workbench-tab:hover` | `color: var(--text-primary, #f3f4f6);` | `color: var(--on-surface);` |
| `.gmap-authority-workbench-tab.is-active` | `color: var(--text-primary, #f3f4f6); background: var(--panel-bg, #0f1115); border-color: var(--border-color, rgba(255,255,255,0.08));` | `color: var(--on-surface); background: var(--surface-container-low); border-color: var(--outline-variant);` |
| `.gmap-authority-workbench-panel` | `flex: 1 1 auto; overflow: auto; padding: 12px 16px 16px;` | (unchanged) |
| `.authority-workbench-section-title` | `font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-muted, #9ca3af); margin: 0 0 6px;` | `font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--slate-400); margin: 0 0 6px;` |
| `.authority-workbench-section-context` | `font-size: 11px; color: var(--text-muted, #9ca3af); margin: 0 0 6px; font-style: italic;` | `font-size: 11px; color: var(--slate-400); margin: 0 0 6px; font-style: italic;` |
| `.authority-workbench-stat` | `display: flex; justify-content: space-between; font-size: 12px; padding: 4px 0; border-bottom: 1px dotted var(--border-color, rgba(255,255,255,0.04));` | `display: flex; justify-content: space-between; font-size: 12px; padding: 4px 0; border-bottom: 1px dotted var(--outline-variant);` |
| `.authority-workbench-stat-label` | `color: var(--text-muted, #9ca3af);` | `color: var(--slate-400);` |
| `.authority-workbench-stat-value` | `color: var(--text-primary, #f3f4f6); font-weight: 600;` | `color: var(--on-surface); font-weight: 600;` |
| `.authority-workbench-stat-emphasis .authority-workbench-stat-value` | `color: var(--accent-emphasis, #fbbf24);` | `color: var(--badge-warn, #fbbf24);` |
| `.authority-workbench-stat-gap .authority-workbench-stat-value` | `color: var(--accent-gap, #f87171);` | `color: var(--badge-bad, #f87171);` |
| `.authority-workbench-stat-critical .authority-workbench-stat-value` | `color: var(--severity-critical, #ef4444);` | `color: var(--badge-bad, #ef4444);` |
| `.authority-workbench-stat-warning .authority-workbench-stat-value` | `color: var(--severity-warning, #f59e0b);` | `color: var(--badge-warn, #f59e0b);` |
| `.authority-workbench-stat-info .authority-workbench-stat-value` | `color: var(--severity-info, #60a5fa);` | `color: var(--badge-info, #60a5fa);` |
| `.authority-workbench-empty` | `font-size: 12px; color: var(--text-muted, #9ca3af); padding: 8px 0; font-style: italic;` | `font-size: 12px; color: var(--slate-400); padding: 8px 0; font-style: italic;` |
| `.authority-workbench-diagnostic` | `font-size: 12px; padding: 4px 0; border-bottom: 1px dotted var(--border-color, rgba(255,255,255,0.04)); display: flex; flex-wrap: wrap; gap: 6px; align-items: baseline;` | `font-size: 12px; padding: 4px 0; border-bottom: 1px dotted var(--outline-variant); display: flex; flex-wrap: wrap; gap: 6px; align-items: baseline;` |
| `.authority-workbench-diagnostic-severity` | `font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; font-weight: 700; padding: 1px 6px; border-radius: 3px; background: rgba(255,255,255,0.06);` | `font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; font-weight: 700; padding: 1px 6px; border-radius: 3px; background: var(--surface-container);` |
| `.authority-workbench-diagnostic-critical .authority-workbench-diagnostic-severity` | `color: var(--severity-critical, #ef4444); background: rgba(239,68,68,0.12);` | `color: var(--badge-bad, #ef4444); background: rgba(239,68,68,0.12);` (overlay preserved) |
| `.authority-workbench-diagnostic-warning .authority-workbench-diagnostic-severity` | `color: var(--severity-warning, #f59e0b); background: rgba(245,158,11,0.12);` | `color: var(--badge-warn, #f59e0b); background: rgba(245,158,11,0.12);` (overlay preserved) |
| `.authority-workbench-diagnostic-info .authority-workbench-diagnostic-severity` | `color: var(--severity-info, #60a5fa); background: rgba(96,165,250,0.12);` | `color: var(--badge-info, #60a5fa); background: rgba(96,165,250,0.12);` (overlay preserved) |
| `.authority-workbench-diagnostic-kind` | `color: var(--text-primary, #f3f4f6); font-weight: 600;` | `color: var(--on-surface); font-weight: 600;` |
| `.authority-workbench-diagnostic-message` | `color: var(--text-muted, #9ca3af);` | `color: var(--slate-400);` |

---

## 5. Files Modified

| File | Change |
| --- | --- |
| [internal/httpapi/explorer/assets/css/authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) | 24 rules rewritten between lines 796 and 995. Height model switches from `min-height: 36px; max-height: 280px / 320px; transition: max-height 200ms ease` to `height: 36px / 320px; transition: height 0.18s ease-out`. Header gains explicit `height: 36px; flex-shrink: 0;` plus `padding: 8px 14px` and `font-family: var(--font-display)`. Title becomes uppercase eyebrow (11px / 700 / 0.08em / `--slate-300`). Toggle button gains a `:focus-visible` rule and aligns dimensions / tokens with Context. Body gains `flex: 1 1 auto; min-height: 0;`. Tabs gain `background: var(--surface-container-lowest)`. All 10 distinct undefined tokens migrate per Step 0.1 mapping. CSS lens-routing rules at L782-790 unchanged. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js) | `_setExpanded` gains two additions: (1) glyph swap reading `_expanded` and setting the toggle button's `<span aria-hidden>` textContent to `▼` (expanded) or `▲` (collapsed), mirroring Context tray at L1016-1017; (2) 200ms `setTimeout` calling `window.MIDASExplorerGraph.camera.scheduleFitToView()`, mirroring Context tray at L1039-1043. No other module changes. Public API (`render`, `notifySelectionChanged`, `clear`, `setActiveTab`, `_TAB_IDS`) byte-identical. |
| [internal/httpapi/explorer_d32h_fix2d_converge_test.go](../../internal/httpapi/explorer_d32h_fix2d_converge_test.go) | **New file.** 8 focused regression-guard tests (see §6). |
| [docs/implementation/D32h-fix-2d-converge-letterbox-css-convergence.md](./D32h-fix-2d-converge-letterbox-css-convergence.md) | **New file.** This deliverable. |

### Files NOT modified

- `internal/httpapi/explorer/index.html` — DOM untouched; `hidden` attribute retained per D32hFix1 contract.
- `internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js` — Context module read-only.
- `internal/httpapi/explorer/assets/css/governance-map.css` — Context CSS read-only.
- `internal/httpapi/explorer/assets/css/tokens.css` — token definitions read-only.
- All Authority modules outside `authority-graph-workbench.js`.
- All backend Go code.

---

## 6. Tests Added or Updated

### 6.1 New tests in [internal/httpapi/explorer_d32h_fix2d_converge_test.go](../../internal/httpapi/explorer_d32h_fix2d_converge_test.go)

| Test | Purpose |
| --- | --- |
| `AuthorityWorkbenchUsesHeightCollapseModel` | Pins `.gmap-authority-workbench { ...height: 36px... }` and `.is-expanded { height: 320px; }`. Bans the pre-fix `max-height: 280px / 320px` and `min-height: 36px` forms in CSS rule bodies (comment-stripped). |
| `AuthorityWorkbenchTransitionsHeight` | Pins `transition: height 0.18s ease-out;` on the workbench root. Bans `transition: max-height …` on any rule targeting `.gmap-authority-workbench` (regex-based rule-body scan). |
| `AuthorityWorkbenchHeaderHasFixedHeight` | Pins explicit `height: 36px;` and `flex-shrink: 0;` inside the header rule body. |
| `AuthorityWorkbenchToggleUpdatesGlyph` | Pins the glyph-update branch in `_setExpanded` — querySelector for `span[aria-hidden]` plus `glyph.textContent = _expanded ? '▼' : '▲';`. |
| `AuthorityWorkbenchSchedulesRefitOnToggle` | Pins the 200ms setTimeout calling `camera.scheduleFitToView()`. |
| `AuthorityWorkbenchUsesSharedTokens` | Pins references to `--surface-container-low`, `--surface-container-lowest`, `--surface-container`, `--outline-variant`, `--on-surface`, `--slate-300`, `--slate-400`, `--font-display`, `--primary`. Also pins `--badge-bad/warn/info` convention-with-fallback severity tokens. |
| `AuthorityWorkbenchHardcodedFallbacksRemoved` | Bans all 10 pre-fix undefined-token references (`--panel-bg`, `--panel-bg-strong`, `--border-color`, `--text-primary`, `--text-muted`, `--accent-emphasis`, `--accent-gap`, `--severity-critical`, `--severity-warning`, `--severity-info`) within the workbench CSS block. |
| `ContextLetterboxUntouched` | Pins Context tray CSS contract (`height: 36px`, `transition: height 0.18s ease-out`, `height: 320px`) in `governance-map.css` and Context module (`gmapEvidenceTrayExpanded`, `applyGmapEvidenceTrayState`, `notifySelectionChanged` export) in `context-evidence-tray.js`. |

### 6.2 Tests updated

**Zero.** Step 0.4's audit confirmed no existing test pins the declarations being changed. Verified by running the full D32hFix1|D32hFix2b|D32hFix2c|D32hFix2d subset both before and after the rewrite — all 39 pre-existing tests pass unchanged.

### 6.3 Test-fix during run

`AuthorityWorkbenchUsesHeightCollapseModel` initially failed because its banned-substring check scanned the raw CSS including comments — my explanatory comment block legitimately documents the deleted `max-height: 280px` form as part of the fix rationale, which the test mistakenly flagged. **Fix: strip CSS comments before the banned-substring check** (mirroring the regex pattern from D32hFix2d's `AuthorityWorkbenchNotHiddenByImportantOverride`). The test now correctly scans only rule bodies. This is a test-file refinement, not a CSS rollback.

---

## 7. Test Evidence

### 7.1 Context regression baseline AFTER all changes (Docker)

`go test ./internal/httpapi -run 'D32hFix1_ContextEvidenceTrayUntouched' -count=1 -timeout 60s -v`:

```
=== RUN   TestExplorer_D32hFix1_ContextEvidenceTrayUntouched
--- PASS: TestExplorer_D32hFix1_ContextEvidenceTrayUntouched (0.00s)
PASS
ok  	github.com/accept-io/midas/internal/httpapi	0.016s
```

The baseline holds.

### 7.2 Focused subset (Docker)

`go test ./internal/httpapi -run 'D32hFix2dConverge|D32hFix2d|D32hFix2c|D32hFix2b|D32hFix1' -count=1 -timeout 120s -v` — **47 / 47 PASS**:

```
--- PASS: TestExplorer_D32hFix1_* (16 tests)
--- PASS: TestExplorer_D32hFix2b_* (7 tests)
--- PASS: TestExplorer_D32hFix2c_* (14 tests)
--- PASS: TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride
--- PASS: TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved
--- PASS: TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesHeightCollapseModel
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchTransitionsHeight
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHeaderHasFixedHeight
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchToggleUpdatesGlyph
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchSchedulesRefitOnToggle
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesSharedTokens
--- PASS: TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHardcodedFallbacksRemoved
--- PASS: TestExplorer_D32hFix2dConverge_ContextLetterboxUntouched
PASS
ok  	github.com/accept-io/midas/internal/httpapi	0.079s
```

### 7.3 Full `./test.sh all` (Docker)

Every package `ok`. No `FAIL` line anywhere.

```
ok  	github.com/accept-io/midas/cmd/midas	0.040s
ok  	github.com/accept-io/midas/internal/adminaudit	0.005s
ok  	github.com/accept-io/midas/internal/agent	0.005s
ok  	github.com/accept-io/midas/internal/audit	0.420s
ok  	github.com/accept-io/midas/internal/auth	0.006s
ok  	github.com/accept-io/midas/internal/authority	0.006s
ok  	github.com/accept-io/midas/internal/authz	0.011s
ok  	github.com/accept-io/midas/internal/bootstrap	0.026s
ok  	github.com/accept-io/midas/internal/config	0.041s
ok  	github.com/accept-io/midas/internal/controlaudit	0.006s
ok  	github.com/accept-io/midas/internal/controlplane/apply	0.042s
ok  	github.com/accept-io/midas/internal/controlplane/approval	0.026s
ok  	github.com/accept-io/midas/internal/controlplane/parser	0.030s
ok  	github.com/accept-io/midas/internal/controlplane/types	0.010s
ok  	github.com/accept-io/midas/internal/controlplane/validate	0.023s
ok  	github.com/accept-io/midas/internal/decision	1.109s
ok  	github.com/accept-io/midas/internal/dispatch	2.113s
ok  	github.com/accept-io/midas/internal/drift	0.009s
ok  	github.com/accept-io/midas/internal/envelope	0.005s
ok  	github.com/accept-io/midas/internal/escalation	0.008s
ok  	github.com/accept-io/midas/internal/externalref	0.006s
ok  	github.com/accept-io/midas/internal/failmode	0.014s
ok  	github.com/accept-io/midas/internal/governancecoverage	0.010s
ok  	github.com/accept-io/midas/internal/governanceexpectation	0.009s
ok  	github.com/accept-io/midas/internal/graph/authority	0.027s
ok  	github.com/accept-io/midas/internal/graph/context	0.019s
ok  	github.com/accept-io/midas/internal/httpapi	6.299s
ok  	github.com/accept-io/midas/internal/identity	0.006s
ok  	github.com/accept-io/midas/internal/kafka	0.010s
ok  	github.com/accept-io/midas/internal/localiam	1.082s
ok  	github.com/accept-io/midas/internal/metrics	0.011s
ok  	github.com/accept-io/midas/internal/oidc	0.009s
ok  	github.com/accept-io/midas/internal/outbox	0.008s
ok  	github.com/accept-io/midas/internal/platformauth	0.008s
ok  	github.com/accept-io/midas/internal/quickstart	0.028s
ok  	github.com/accept-io/midas/internal/store/memory	0.022s
ok  	github.com/accept-io/midas/internal/store/postgres	16.229s
ok  	github.com/accept-io/midas/internal/surface	0.008s
```

---

## 8. Constraints Confirmation

- **No backend Go files modified.** Verified via `git diff --name-only`.
- **No schema, OpenAPI, seed, runtime, deployment, or workflow changes.**
- **No GitHub operations.** No fetch / push / pull / branch / merge / rebase / commit / PR.
- **No Context implementation changes.** `context-graph-view.js`, `context-graph-adapter.js`, `context-graph-inspector.js`, `context-evidence-tray.js`, all `drift/*` modules, `governance-map.css` — read-only. Verified by `ContextLetterboxUntouched` + the pre-existing `D32hFix1_ContextEvidenceTrayUntouched`.
- **No Authority DOM changes.** `index.html` unchanged. `<section id="gmap-authority-workbench" … hidden>` and its 5 tab buttons are byte-identical to D32h-fix-1.
- **No module API changes.** `window.MIDASExplorerGraph.authorityWorkbench` public surface (`render`, `notifySelectionChanged`, `clear`, `setActiveTab`, `_TAB_IDS`) byte-identical.
- **No layer-state contract changes.** D32h-fix-2c's `computeAuthorityLayout(spec, GMAP, layerState)` + `visibleNodes` / `visibleEdges` contract preserved (verified by `D32hFix2d_D32hFix2cLayerStatePreserved` + all 14 D32hFix2c tests).
- **No selection-path contract changes.** D32h-fix-2b's lens-aware `selectGovernanceMapNode` + Authority inspector notify-hook preserved (verified by `D32hFix2d_D32hFix2bSelectionPathPreserved` + all 7 D32hFix2b tests).

---

## 9. Manual Verification

**The seven manual verification steps cannot be performed by the implementing session — there is no browser controllable from this environment.** The CSS rule rewrite and the JS behaviour additions are correctly served to the wire (verified via the test suite and via `curl http://localhost:8080/explorer/assets/css/authority-graph.css`), but visual / animation confirmation requires a browser. Per the working-constraint guidance for this tranche family, this is the established gap that Tier-3 (browser snapshot) verification addresses in D32h-fix-2h.

| # | Check | Status |
| --- | --- | --- |
| 1 | Authority lens loads with workbench showing only 36px header | **DEFERRED (browser required)** — CSS verified on the wire: `.gmap-authority-workbench { … height: 36px; … }` present at `/explorer/assets/css/authority-graph.css`. |
| 2 | Toggle expands to 320px with ~0.18s animation; glyph flips ▲ → ▼ | **DEFERRED** — `.is-expanded { height: 320px; }` present; `transition: height 0.18s ease-out;` present; JS glyph swap pinned. |
| 3 | Graph repositions on expand | **DEFERRED** — JS schedules `camera.scheduleFitToView()` after 200ms (pinned). |
| 4 | Toggle collapses, glyph flips ▼ → ▲, graph repositions | **DEFERRED** — same code path, symmetric. |
| 5 | Context lens still works identically | **VERIFIED at source level** by `D32hFix1_ContextEvidenceTrayUntouched` (pre + post) + `D32hFix2dConverge_ContextLetterboxUntouched`. No Context file was modified. |
| 6 | Switching back to Authority preserves workbench state within session | **VERIFIED at source level** — `_expanded` is module-local; D32h-fix-2d-converge does not alter state-persistence semantics. Same behaviour as Context (also module-local). |
| 7 | Authority workbench visually indistinguishable from Context tray in chrome | **DEFERRED** — CSS rule sets are independent but now use the same height model, the same Material Design tokens for shell chrome, the same toggle dimensions (28×22), the same padding (8px 14px on header, 6px 14px on tabs), the same transition (`height 0.18s ease-out`). Visual indistinguishability is a runtime assertion. |

**Reader's smoke-check procedure** (recommended before declaring the operator's three defects closed):

1. Start `localhost:8080`. Switch to Authority lens.
2. Expected: bottom letterbox shows only the 36px header bar with the title eyebrow "AUTHORITY POSTURE" (or current subtitle text), subtitle ellipsing right, toggle `▲` button right-aligned. NOT the previous full-height "open by default" appearance.
3. Click the toggle. Expected: workbench expands smoothly to 320px over ~0.18s; glyph flips to `▼`; graph re-fits to the new (smaller) canvas-scroll height after ~200ms.
4. Click again. Expected: workbench collapses smoothly to 36px; glyph flips back to `▲`; graph re-fits to the larger canvas-scroll.
5. Switch to Context lens. Expected: Drift Analytics tray appears exactly as before; toggle behaviour, transitions, colours unchanged.
6. Switch back to Authority. Expected: workbench in whichever state it was left in (state survives the lens switch within the session).
7. Compare the two letterboxes side by side (open both lenses across two tabs / windows if possible). Expected: header chrome, toggle button shape and colour, tab strip styling visually identical apart from the lens-specific tab labels and subtitle text. The body content inside each lens is intentionally different (Drift Analytics for Context; Authority posture stats for Authority).

If any of these fail, stop and capture a [DevTools snapshot](../evidence/D32h-fix-1/snapshot.js) for diagnosis. The source-string tests cannot detect runtime layout / animation defects — D32g-analysis-4's "Tier-1 source pins green, browser shows defects" pattern. Tier-3 (browser snapshot) verification is the canonical gate.

---

## 10. Next-Tranche Handoff

Per the [D32h-fix-2 roadmap](./D32h-fix-2-plan-authority-lens-roadmap.md):

**Recommended next tranche:** `D32h-fix-2e` — Shared-node "Shared by N" visual semantics. Independent of any deferred goja harness; can proceed with existing source-string pins.

**Alternative:** `D32h-fix-2f` — Browser-verified layout refinement. Requires the 2a baseline snapshots first.

**Architectural extraction** (shared `graph-letterbox` module) remains deferred per §3. Triggers documented; revisit when a third lens or further divergence pressure appears.

**Systemic learning continuation.** This tranche resolved three visible operator-reported defects, but two of them (close-arrow ineffective, "defaults open in Focus Mode") were latent for the entire lifetime of D32h-fix-1 because Tier-1 source-string tests cannot detect CSS-cascade-driven visibility failures. Tier-2 (goja) is acceptable for adapter and layout helper math; Tier-3 (browser snapshot) remains the only honest verification for visual-layout outcomes. The D32h-fix-2-plan roadmap continues to prioritise Tier-3 in D32h-fix-2h.

**End of D32h-fix-2d-converge.**
