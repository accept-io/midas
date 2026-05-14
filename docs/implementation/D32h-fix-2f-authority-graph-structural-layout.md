# D32h-fix-2f — Authority Graph Structural Layout Implementation

## Tranche

D32h-fix-2f. Authority Graph Structural Layout. Local work only; no
backend, schema, OpenAPI, seed, runtime, deployment, or GitHub
workflow changes; no new dependencies; no commits.

## Scope (delivered)

Spec-aligned structural layout primitives behind the Authority Graph
lens, per the design specification at
[docs/design/D32h-authority-graph-design-specification.md](../design/D32h-authority-graph-design-specification.md):

- Layout constants migrated from fixed-y literals to derived
  expressions rooted in named constants (§5.2 / §5.5).
- Lane stride driven by the spec-named `AUTHORITY_LANE_GAP`.
- Y anchors derived from `AUTHORITY_TOP_MARGIN + n *
  AUTHORITY_VERTICAL_STEP`.
- Business Service centred above lanes (§5.4).
- `missingBelow` structural metadata propagated from layout helper to
  DOM via `data-missing-below` (no styled badge; §5.7).
- `sharedBy` structural metadata propagated from layout helper to DOM
  via `data-shared-by` (no styled badge; §5.8).
- Centroid fallback for shared spine nodes: distance threshold
  `1.5 * (NODE_W + AUTHORITY_LANE_GAP)` OR same-level collision
  (§5.9 + user-approved clarification).
- Same-level collision detection: two nodes at the same `y` collide if
  their `x` ranges overlap (`|x1 - x2| < NODE_W`). Detection runs
  before the shared-node placement commits its position.
- `canvasH` tail uses `AUTHORITY_BOTTOM_MARGIN` (§5.2 / §15).
- Visual semantics out of scope (deferred to D32h-fix-2e).

## Files touched

| File | Change |
|---|---|
| [internal/httpapi/explorer/assets/js/governance-map/constants.js](../../internal/httpapi/explorer/assets/js/governance-map/constants.js) | Added `AUTHORITY_TOP_MARGIN`, `AUTHORITY_VERTICAL_STEP`, `AUTHORITY_BOTTOM_MARGIN`, `AUTHORITY_LANE_GAP`. `AUTHORITY_LAYERS` table values derived from `TOP_MARGIN + n * VERTICAL_STEP`. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) | `_layerY` fallback now derives from new constants; layout helper consumes `AUTHORITY_LANE_GAP` for stride and `AUTHORITY_BOTTOM_MARGIN` for canvas tail; added `nearestOwnerX`, `leftmostOwnerX`, `collidesAtLevel`, `resolveSharedX`, `recordSharedBy`; missingBelow / sharedBy metadata attached to visibleNode entries; sidecar geometry unchanged. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | `_paintNode` accepts a sixth `visibleEntry` argument; emits `data-missing-below` and `data-shared-by` attributes when those fields are populated on the entry. |
| [internal/httpapi/explorer_d32h_fix2f_test.go](../../internal/httpapi/explorer_d32h_fix2f_test.go) | New focused source-string test file. 28 tests covering positive contracts + negative pins. |
| [internal/httpapi/explorer_d32h_fix2c_test.go](../../internal/httpapi/explorer_d32h_fix2c_test.go) | Updated one pin string (`_paintNode` call now passes the visibleEntry argument). Per Step 0.7 audit + user instruction to update pin strings rather than silence tests. |

No CSS changes. No HTML changes. No adapter changes. No drawer or
workbench changes. No selection-path changes. No Context-lens changes.

## Constants table

| Constant | Value | Source |
|---|---|---|
| `GMAP.AUTHORITY_TOP_MARGIN` | `40` | New (spec §5.2) |
| `GMAP.AUTHORITY_VERTICAL_STEP` | `GMAP.NODE_H + 40` = `104` | New (spec §5.5) |
| `GMAP.AUTHORITY_BOTTOM_MARGIN` | `60` | New (spec §5.2 / §15) |
| `GMAP.AUTHORITY_LANE_GAP` | `GMAP.AUTHORITY_CHAIN_GAP` (alias) = `48` | New name; spec §5.2 |
| `GMAP.AUTHORITY_CHAIN_GAP` | `48` | Preserved (D32h-impl-1 pin) |
| `GMAP.AUTHORITY_SIDECAR_GAP` | `36` | Preserved (D32h-impl-1 pin; spec §5.2 fallback `NODE_W/2 = 110` rejected — 36 is the authoritative Authority sidecar geometry) |
| `GMAP.AUTHORITY_LAYERS.BUSINESS.y` | `40` (was `24`) | Derived |
| `GMAP.AUTHORITY_LAYERS.SURFACE.y` | `144` (unchanged value, now derived) | Derived |
| `GMAP.AUTHORITY_LAYERS.PROFILE.y` | `248` (was `264`) | Derived |
| `GMAP.AUTHORITY_LAYERS.GRANT.y` | `352` (was `384`) | Derived |
| `GMAP.AUTHORITY_LAYERS.AGENT.y` | `456` (was `504`) | Derived |

Visual rhythm tightens by 16 px per row (step 120 → 104). The
`AUTHORITY_LAYERS` table object is preserved — only its values are now
derived — so the D32h-impl-1 contract pin on the key names is
unaffected.

## Algorithm summary

### Lane assignment (spec §5.4)

```
chainX[chain.chainId] = EDGE_PAD + i * (NODE_W + AUTHORITY_LANE_GAP)
```

Chain ordering follows the adapter's `byKind.decision_surface` walk
(backend emission order, deterministic per the D32h-impl-1 contract).

### Business Service centring (spec §5.4)

```
rootX = (chainX[chainOrder[0]] + chainX[chainOrder[last]]) / 2
```

When no chains exist (sparse projection), root is centred in the
minimum canvas width.

### Vertical alignment (spec §5.5)

```
y_i = AUTHORITY_TOP_MARGIN + i * AUTHORITY_VERTICAL_STEP
```

`i ∈ { BUSINESS:0, SURFACE:1, PROFILE:2, GRANT:3, AGENT:4 }`.

### Shared-node placement (spec §5.8 / §5.9 + user-approved clarification)

For each shared profile / grant / agent:

```
cx     = centroid(chainX[owner_chain_ids])
near   = nearest_owner_x(cx)
trip_a = |cx - near| > 1.5 * (NODE_W + AUTHORITY_LANE_GAP)
trip_b = collides_at_level(cx, level_y, exclude_self)
if (trip_a || trip_b) placement = leftmost_owner_x()
else                  placement = cx
sharedBy[refKey] = owner_chain_ids.length  // when > 1
```

`collides_at_level(x, y, exclude)` returns true if any already-placed
position at the same `y` has `|x - other.x| < NODE_W`. Detection
operates on the live `positions` map, so it correctly excludes the
node being placed (via `excludeKey`) and catches collisions against
both spine nodes already in the same level and any other prior
shared-node placements at the same level.

### Sidecar placement (spec §5.10 — preserved)

For each governance owner:

```
sidecar_slot = (owner.x + NODE_W + AUTHORITY_SIDECAR_GAP, owner.y)
```

Slot collisions offset vertically by `NODE_H + 16`.

### Missing-link metadata (spec §5.7)

For each chain, the upstream card holding the truncation marker is
identified before pushVisibleNode runs:

```
if !chain.profile:        missingBelow[chain.surface.refKey] = 'profile'
else if !chain.grant:     missingBelow[chain.profile.refKey] = 'grant'
else if !chain.agent:     missingBelow[chain.grant.refKey]   = 'agent'
```

The view writes the value as `data-missing-below` on the painted
card. No badge styling in this tranche.

### Canvas bounds (spec §15)

```
canvasW = max(MIN_CANVAS_W, maxX + NODE_W + EDGE_PAD)
canvasH = max(CANVAS_H,     maxY + NODE_H + AUTHORITY_BOTTOM_MARGIN)
```

`maxX` / `maxY` iterate `visibleNodes` only (D32h-fix-2c contract
preserved).

## Tests

### Focused source-string tests (Tier-1)

[internal/httpapi/explorer_d32h_fix2f_test.go](../../internal/httpapi/explorer_d32h_fix2f_test.go) — 28 tests.

Positive contracts:

1. `ConstantsDeclareAuthorityLaneGap`
2. `ConstantsDeclareVerticalStep`
3. `ConstantsDeclareTopAndBottomMargin`
4. `AuthorityLayersDerivedFromConstants`
5. `AuthoritySidecarGapPreserved`
6. `LayoutHelperConsumesLaneGap`
7. `LayoutHelperDerivesYFromConstants`
8. `LayoutHelperConsumesBottomMargin`
9. `BusinessServiceCentredAboveLanes`
10. `VisibleNodeCarriesMissingBelow`
11. `VisibleNodeCarriesSharedBy`
12. `CentroidFallbackDistanceThreshold`
13. `CentroidFallbackSameLevelCollision`
14. `ResolveSharedXReturnsFallbackOrCentroid`
15. `SidecarOwnerRelativePlacement`
16. `SidecarCollisionOffset`
17. `ViewPropagatesDataMissingBelow`
18. `ViewPropagatesDataSharedBy`
19. `ViewForwardsVisibleEntryToPaintNode`
20. `VisibleNodesVisibleEdgesContractPreserved`
21. `CoordinateContractPreserved`
22. `D32gFix3InvariantsPreserved`
23. `SpineAnchorPairPreserved`
24. `LayoutHelperSignaturePreserved`
25. `EmptySpecHandlingPreserved`

Negative pins:

26. `NoHardcodedYInLayoutHelper`
27. `NoCategoryColourCSSAdded`
28. `NoNewBadgeCSSClasses`

Plus the byte-identical contract pins:

- `AdapterSignatureByteIdentical`
- `SelectionPathByteIdentical`

### Test runs

| Run | Result |
|---|---|
| Pre-implementation baseline (`D32hFix1_ContextEvidenceTrayUntouched`) | **PASS** |
| Focused suite (`D32hFix2f\|D32hFix2dConverge\|D32hFix2d\|D32hFix2c\|D32hFix2b\|D32hFix1`) post-implementation | **PASS** (all 75 tests across the matched suites) |
| Full suite (`./test.sh all`) | **PASS** (Docker harness; green "Tests complete" marker) |
| Post-implementation baseline regression (`D32hFix1_ContextEvidenceTrayUntouched`) | **PASS** |

One pin update was required during implementation: the D32h-fix-2c
test pin for `_paintNode(ventry.node, vpos, renderer, adapter,
overlays);` was updated to include the new sixth argument
(`ventry`). Per the Step 0.7 audit guidance + the user-stated rule
"Do not silence existing tests by deleting them. Update pin-strings
to match the new contract." The intent of the original pin — that the
view paints from visibleNodes — is preserved verbatim; only the
argument count is updated.

## Manual browser verification

**Not performed in this environment.** The agent has no browser
control in this session. The 16 visual checks at design spec §16.1
are the gating criteria for visual no-overlap and centroid-fallback
readability. They are listed below for completeness with PASS/FAIL
deferred to a manual session.

### Required checks (deferred to manual verification session)

For each canonical service (Authority Graph Showcase = dense; a
sparse service; a medium-realistic service; the medium-realistic
service under Context lens):

| Check | Method | Status |
|---|---|---|
| Default fail-mode and escalation hidden | Snapshot canvas with layers off; assert no FMP / escalation nodes present | **deferred** |
| Authority spine readable | For each chain, surface/profile/grant/agent x within 4 px of each other | **deferred** |
| Lanes obvious | `chainX` strictly monotonic across chains | **deferred** |
| No node overlaps | No two visible nodes have overlapping bounding boxes | **deferred** |
| No connectors through node boxes | No connector path intersects a node bounding box | **deferred** |
| Missing-link metadata present | For surfaces with truncated chain, `data-missing-below` attribute set | **deferred** |
| Shared-node metadata present | For nodes with multiple owners, `data-shared-by` set with N | **deferred** |
| Category colours applied | Each node's left border matches kind category | **D32h-fix-2e scope** |
| Fail-mode layer on: sidecars attach | Each FMP node's x > owner.x + NODE_W | **deferred** |
| Escalation layer on: sidecars attach | Each escalation node's x > profile.x + NODE_W | **deferred** |
| Workbench visible in Authority lens | `#gmap-authority-workbench` visible | **deferred** |
| Drift Analytics visible in Context lens | `#gmap-evidence-tray` visible, Authority workbench hidden | **deferred** |
| No canvas clipping by bottom letterbox | `meta.nodesClippedByBottomRail.length === 0` | **deferred** |
| Selected-node contrast readable | `selected.subElements[*].color` matches design tokens | **D32h-fix-2e scope** |
| Business service centred above lanes | `bs.x ≈ (chain[0].x + chain[last].x) / 2` | **deferred** |
| Context lens unchanged | Context canvas + drift analytics render identically to pre-tranche | **deferred** |

### Snapshot evidence affordance

[docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js)
is the existing DevTools snippet that captures the runtime DOM state
needed for the per-check verification. The structural metadata added
in this tranche (`data-missing-below`, `data-shared-by`) flows
directly into the snapshot output via the existing data-attribute
collection.

If the user wants to run the visual verification now, they can:

1. Boot the Explorer locally.
2. Open the Authority Graph Showcase service.
3. Paste the snippet into DevTools.
4. Repeat for a sparse service, a medium service, and the
   medium-realistic service under Context lens.
5. Compare snapshots against the 16 checks above.

Or schedule a follow-up tranche dedicated to browser-evidence-driven
threshold tuning (the user's approval already covered this:
"Refine [the threshold] via browser evidence in a later tranche if
needed").

## Spec acceptance criteria (§20) — status

| # | Criterion | Status |
|---|---|---|
| 1 | Authority uses the shared graph shell | Preserved (no shell changes) |
| 2 | Default Authority canvas shows the authority spine only | Preserved (D32h-fix-2c layer-state contract) |
| 3 | Fail-mode and escalation nodes hidden by default | Preserved |
| 4 | Optional governance nodes available through layers | Preserved |
| 5 | Hidden optional governance nodes do not shape default canvas bounds | Preserved (visibleNodes-only bounds derivation) |
| 6 | No nodes overlap on first render | **Structurally enforced** by the layout: distinct lane x per chain; derived y per level; centroid fallback + same-level collision for shared nodes; sidecar slot collision offset. **Visual confirmation deferred to manual browser verification.** |
| 7 | No connectors pass through node bounding boxes on first render | **Structurally enforced** by vertical-lane layout: spine edges go top-to-bottom inside a lane; sidecar edges go horizontally to an offset slot. **Visual confirmation deferred.** |
| 8 | Category colour coding applied per §4.1 | **Deferred to D32h-fix-2e** |
| 9 | Connector line colours applied per §4.2 | **Deferred to D32h-fix-2e** |
| 10 | Missing links visible as badges | **Partial** — structural metadata `data-missing-below` emitted; styled badge deferred to D32h-fix-2e |
| 11 | Shared nodes badged with "Shared by N" | **Partial** — structural metadata `data-shared-by` emitted; styled badge deferred to D32h-fix-2e |
| 12 | Right letterbox shows Authority inspector / diagnostics / posture / help | Preserved (no drawer changes) |
| 13 | Bottom letterbox shows Authority Workbench in Authority lens | Preserved (D32h-fix-2d / D32h-fix-2d-converge) |
| 14 | Context mode still shows Drift Analytics | Preserved (baseline regression confirmed PASS) |
| 15 | Authority selection updates both letterboxes | Preserved (D32h-fix-2b contract) |
| 16 | Workbench content is projection-backed | Preserved |
| 17 | Evidence tab does not fake runtime evidence | Preserved |
| 18 | Letterbox chrome matches Context's contract | Preserved (D32h-fix-2d-converge) |
| 19 | Context Graph not regressed | **Confirmed** via baseline regression run |
| 20 | Tests pass | **Confirmed** via focused suite + full ./test.sh all |
| 21 | Browser verification confirms readability across all four canonical services | **Deferred to manual verification** |

## Deviations from the design specification

None for the structural contract. Two intentional deviations carried
over from prior tranches and explicitly preserved in this one:

- **`AUTHORITY_SIDECAR_GAP = 36`** rather than the spec §5.2 fallback
  `NODE_W / 2 = 110`. The smaller gap is the authoritative Authority
  sidecar geometry per the user-approved Step 0 sign-off.
- **`AUTHORITY_VERTICAL_STEP = NODE_H + 40 = 104`** matches the spec
  derivation exactly; the resulting y-anchor values (40 / 144 / 248 /
  352 / 456) replace the pre-tranche 24 / 144 / 264 / 384 / 504. The
  visual rhythm tightens by 16 px per row. User-approved.

## Tranche table position (§17)

The user's tranche prompt is labelled D32h-fix-2f. The design
specification §17 table lists D32h-fix-2e as "Authority Graph
Structural Layout" and D32h-fix-2f as "Browser-observed layout
refinement". This tranche treats the user's prompt as authoritative
and ships under D32h-fix-2f. Subsequent visual-semantics work
(category colours, badges, hover states) ships under D32h-fix-2e per
the user's explicit scope correction.

## Constraints honoured

- No GitHub interaction. No PRs, pushes, pulls, branches, merges,
  rebases, commits.
- No backend, schema, OpenAPI, seed, runtime, deployment, or
  workflow changes.
- No new dependencies (no goja, no JSDOM, no browser driver).
- No Context lens changes (Context CSS, view, adapter, inspector,
  evidence-tray, drift modules all untouched).
- No category colour CSS, no connector colour styling, no styled
  badges, no hover/selection ring colour edits (deferred to
  D32h-fix-2e).
- Tests run via `./test.sh` in Docker (per user memory).

## Memory and ongoing context

No new memories warranted by this tranche. Existing memories
`project_midas_state.md`, `feedback_testing.md`,
`feedback_commits.md` continue to apply.
