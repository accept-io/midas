# Lucide icons — vendored subset for MIDAS Explorer

This directory contains a **curated subset** of icons from the Lucide
project (https://lucide.dev/), vendored locally for MIDAS Explorer
iconography. **This is not the full Lucide library** — only the icons
listed in [VENDOR.md](./VENDOR.md) are present.

## Status

**D33a-spike-2g-impl-1 vendoring foundation.** Assets only. The icons
are **not yet wired into any runtime** — the planned `midas-icons.js`
registry and the consuming Cytoscape themes / chrome buttons will
land in subsequent tranches (D33a-spike-2g-impl-2 onwards).

You can find the planning context in:

- [docs/implementation/D33a-spike-2g-midas-graph-object-classification-and-icon-catalogue.md](../../../../../../docs/implementation/D33a-spike-2g-midas-graph-object-classification-and-icon-catalogue.md)
  — the full graph object classification and icon catalogue.
- [docs/implementation/D33a-spike-2f-midas-icon-vocabulary-and-lucide-vendoring-plan.md](../../../../../../docs/implementation/D33a-spike-2f-midas-icon-vocabulary-and-lucide-vendoring-plan.md)
  — the original Lucide vendoring plan.
- [docs/implementation/D33a-spike-2g-impl-1-finalise-icon-subset-and-vendor-lucide-svgs.md](../../../../../../docs/implementation/D33a-spike-2g-impl-1-finalise-icon-subset-and-vendor-lucide-svgs.md)
  — this tranche's report.

## What's here

| File | Purpose |
|---|---|
| `LICENSE` | The Lucide upstream LICENSE — includes both the ISC section and the Feather-derived MIT section. Both must be preserved verbatim. |
| `VENDOR.md` | Provenance metadata: source URL, version/tag, vendoring date, icon table, modification policy. |
| `*.svg` | 30 self-contained SVG icon files. Each is a complete `<svg>` element with `stroke="currentColor"` so it adapts to the consumer's CSS `color` token. |

## Why a curated subset (not the full library)

- **Reduced surface for review.** A 30-icon subset can be visually
  reviewed in one sitting; the full Lucide library cannot.
- **No runtime fetches.** Every icon is an embedded static asset; the
  Explorer never reaches a CDN.
- **License audit clarity.** The
  [Lucide LICENSE file](./LICENSE) covers every shipped icon. Adding
  a new icon means a documented catalogue change, not an opaque
  dependency bump.

## How to add another icon later

1. Confirm the concept is documented in the
   [graph object classification catalogue](../../../../../../docs/implementation/D33a-spike-2g-midas-graph-object-classification-and-icon-catalogue.md).
2. Identify the upstream Lucide filename and copy the `.svg` file
   into this directory. Do **not** edit the icon geometry; only the
   modifications permitted in [VENDOR.md](./VENDOR.md) are allowed.
3. Update [VENDOR.md](./VENDOR.md) with the new row in the icon
   table.
4. Update the icon registry (once that lands in a future tranche).
5. Update the test
   `internal/httpapi/explorer_d33a_spike2g_impl1_test.go` to include
   the new filename in `D33aSpike2gImpl1_LucideExpectedIconFilesPresent`
   and bump the expected count.

## Licence

See [LICENSE](./LICENSE). Lucide is distributed under the ISC
License with a Feather-derived MIT section. **Both sections are
preserved here**, and a copy is referenced from the project root
`NOTICE` file.

## Asset-hygiene policy

- **No CDN, no icon font, no remote icon assets.** All icons are
  embedded static SVGs. The Explorer is served via
  `//go:embed explorer` from `internal/httpapi/explorer.go`; these
  files are bundled at build time.
- **No external image, font, or script references inside the SVGs.**
  Only the SVG XML namespace (`http://www.w3.org/2000/svg`) URL is
  allowed.
- **`stroke="currentColor"` everywhere**, so icons recolour via CSS
  tokens (and via the future registry's `cytoscapeDataURI` baker
  for Cytoscape consumption).
- Modifications permitted in [VENDOR.md](./VENDOR.md) are minimal —
  preserve icon geometry; do not change icon meaning.
