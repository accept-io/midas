# Lucide vendor provenance

This file records the authoritative provenance of the Lucide icons
vendored under this directory.

## Source

- **Project**: Lucide
- **Upstream site**: https://lucide.dev/
- **Upstream repository**: https://github.com/lucide-icons/lucide
- **Licence**: ISC (Lucide Icons and Contributors), with a Feather-
  derived MIT section reproduced in [LICENSE](./LICENSE).
- **Source version / tag / commit**: **TO BE VERIFIED BEFORE MERGE.**
  This tranche (D33a-spike-2g-impl-1) lands the vendoring foundation
  in an environment without direct upstream network access. The SVG
  content authored here matches the named Lucide icons' geometry and
  intent based on the known Lucide design vocabulary, but each file
  must be compared against the chosen upstream Lucide tag (e.g.
  `v0.479.0`) before merge. A reviewer should:
  1. Pick a specific upstream tag.
  2. Replace each `*.svg` file with the upstream content **only if
     the upstream version differs materially** from the file present
     here.
  3. Record the chosen tag in this file under "Source version / tag
     / commit" before merge.
- **Vendored date**: 2026-05-15 (date of initial creation; update on
  re-vendoring).

## Modification policy

The following modifications are **permitted** when vendoring an
upstream Lucide SVG:

- Replacing literal stroke colours with `stroke="currentColor"` if
  the upstream file does not already use it.
- Removing `width="24"` and `height="24"` attributes if the consumer
  prefers to set sizes via CSS (this tranche keeps them for
  consistency with upstream Lucide).
- Adding `aria-hidden="true"` if the consumer renders the SVG inline
  without an explicit label.
- Minifying (stripping whitespace and XML comments) so long as the
  tests in `internal/httpapi/explorer_d33a_spike2g_impl1_test.go`
  remain green.

The following modifications are **NOT permitted** without a
documented catalogue change:

- Changing the icon's geometry / path content.
- Removing or replacing strokes / shapes.
- Adding new shapes or annotations.
- Changing the icon's meaning.
- Inlining external content or fonts.

## Attribution note

Some Lucide icons are **derived from Feather** (Cole Bemis, MIT).
The full Lucide LICENSE file in this directory reproduces both:

- the ISC License covering Lucide Icons and Contributors, and
- the MIT License covering Feather-derived icons.

Do not separate the two sections.

The project root [`NOTICE`](../../../../../../NOTICE) file attributes
Lucide and points back to this directory.

## Vendored icon table

30 icons. Categories follow the
[D33a-spike-2g graph object classification catalogue](../../../../../../docs/implementation/D33a-spike-2g-midas-graph-object-classification-and-icon-catalogue.md).

### Authority + shared graph node icons (7)

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `authorityBusinessService` | `building-2.svg` | business_service | Authority + shared | Reused by Context lens |
| `authorityDecisionSurface` | `workflow.svg` | decision_surface | Authority + shared | Reused by Context lens |
| `authorityProfile` | `shield-check.svg` | authority_profile | Authority + shared | Reused by Context lens; `authority_summary` Context rollup also uses this icon |
| `authorityGrant` | `file-check-2.svg` | authority_grant | Authority | |
| `authorityAgent` | `bot.svg` | agent | Authority + shared | Reused by Context lens |
| `authorityFailModePolicy` | `triangle-alert.svg` | fail_mode_policy | Authority | Also used by `severityWarning` |
| `authorityEscalationTarget` | `arrow-up-from-line.svg` | escalation_target | Authority | |

### Context-only graph node icons (5)

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `contextCapability` | `puzzle.svg` | capability | Context | |
| `contextProcess` | `route.svg` | process | Context | |
| `contextAiSystem` | `cpu.svg` | ai_system | Context | Reuse target for future Knowledge model |
| `contextCoverage` | `target.svg` | coverage | Context | |
| `contextAiSystemBinding` | `link-2.svg` | ai_system_binding | Context | |

### Workbench / chrome icons (6)

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `graphRefresh` | `refresh-cw.svg` | refresh action | Workbench | Replaces the `⟳` Unicode glyph in index.html L1302 |
| `chromeSettings` | `settings.svg` | settings nav | Chrome | Replaces `&#9881;` Unicode glyph in index.html L170 |
| `chromeInfo` | `info.svg` | info chip | Chrome | Standardises ad-hoc `ℹ` glyphs |
| `chromeHelp` | `circle-help.svg` | help button | Chrome | |
| `chromeExternal` | `external-link.svg` | external link annotation | Chrome | |
| `chromeDownload` | `download.svg` | download action | Chrome | Replaces `⇣` Unicode glyph in records audit download |

### Lifecycle icons (5)

Surface lifecycle states from `internal/surface/surface.go:18-30`.

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `lifecycleDraft` | `circle-dashed.svg` | draft | Lifecycle | |
| `lifecycleReview` | `eye.svg` | review | Lifecycle | |
| `lifecycleActive` | `circle-check.svg` | active | Lifecycle | Shared with `stateActive` and `severitySuccess` |
| `lifecycleDeprecated` | `archive.svg` | deprecated | Lifecycle | |
| `lifecycleRetired` | `archive-x.svg` | retired | Lifecycle | |

### Severity / operational state icons (3)

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `severityCritical` | `octagon-alert.svg` | critical severity | Severity | Stop-sign octagon distinguishes from warning triangle |
| `stateBlocked` | `circle-x.svg` | agent blocked | Operational state | |
| `stateSuspended` | `circle-pause.svg` | agent suspended | Operational state | |

### Audit / integrity icons (2)

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `auditIntegrityVerified` | `lock.svg` | hash-chain integrity verified | Audit | From `internal/audit/integrity.go` |
| `auditIntegrityBroken` | `lock-open.svg` | hash-chain integrity broken | Audit | Operator-critical; icon + colour + text required |

### Posture icons (2)

Note: this file uses the corrected `posture*` prefix
(D33a-spike-2g-impl-1 correction; earlier catalogue used the
`postur*` typo).

| MIDAS-facing key | Lucide filename | MIDAS concept | Category | Notes |
|---|---|---|---|---|
| `postureDrift` | `trending-down.svg` | drift signal | Posture | Backend at `internal/drift/` |
| `postureDiagnostics` | `stethoscope.svg` | diagnostics severity index | Posture | Per-node diagnostic rollup |

**Total: 30 icons.**

## Deferred / future candidates

These concepts are documented in the
[D33a-spike-2g catalogue](../../../../../../docs/implementation/D33a-spike-2g-midas-graph-object-classification-and-icon-catalogue.md)
but not vendored in this tranche because the corresponding backend
or UI surface does not yet exist:

- All Knowledge Graph icons (`knowledgePolicy`, `knowledgeRule`,
  `knowledgeObligation`, `knowledgeDocument`, `knowledgeFramework`,
  etc.).
- `postureResilience`, `postureRuntimeEvent` — no Go package.
- `graphFilter`, `chromeMore` — no UI feature today.
- `graphPan` / `graphSelect` / `graphFocus` / `graphFit` /
  `graphZoomIn` / `graphZoomOut` / `graphLayers` / `graphSearch` —
  high-quality bespoke inline SVGs already exist in index.html;
  Lucide swap is per-icon discretion only.

When any of these concepts mature, follow the "How to add another
icon later" checklist in [README.md](./README.md).
