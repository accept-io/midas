# D33a-spike-2g-impl-4b — Thin Card Icon Title Spacing Calibration

This report documents the native Cytoscape calibration pass for icon/title spacing in the Authority thin-card PoC.

The calibration uses `text-halign` with the value `'right'` plus `text-margin-x` so the title lands on a fixed internal anchor rather than drifting with label length.

The remaining mixed typography limitation is still accepted for the native Cytoscape PoC. This tranche does not change production Authority rendering.

