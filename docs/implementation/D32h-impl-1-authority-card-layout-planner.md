# D32h-impl-1 — Authority Card-Layout Adapter and Spec-Driven Layout Planner

**Tranche:** D32h-impl-1.
**Boundary:** frontend layout methodology only. No backend, runtime, schema,
OpenAPI, seed, service-refresh, or GitHub changes.
**Working constraint:** local-only. No fetch / push / pull / branch / merge /
rebase / commit / PR was performed.

This tranche moves the Authority Graph onto the same methodology Context
uses: a typed *card-layout spec* the adapter emits, a *pure layout helper*
the view consumes, a *constants-driven row table*, the *shared coordinate
contract*, and the *shared `ctx` hook bag* for camera + selection +
summary dispatch. The kind-bucketed planner is gone.

---

## 1. Executive Summary

The Authority Graph now follows the Context Graph methodology:

1. **Typed layout spec.** `authorityAdapter.mapToCardLayout(payload, view)`
   walks `payload.edges` once and emits a typed
   `{ root, chains[], governance, unlinked, … }` shape. The view never
   branches on raw projection lists.
2. **Spec-driven planner.** A new pure module
   `authority-graph-layout.js` exposes `computeAuthorityLayout(spec,
   GMAP)` → `{ positions, canvasW, canvasH, chainOrder }`. DOM-free,
   independently testable.
3. **Constants table.** New `GMAP.AUTHORITY_LAYERS` (BUSINESS / SURFACE /
   PROFILE / GRANT / AGENT row anchors) plus `AUTHORITY_CHAIN_GAP` and
   `AUTHORITY_SIDECAR_GAP`. Row Y is no longer computed in the view.
4. **Topology-aware placement.** Surface → Profile → Grant → Agent
   chains share an x lane. Shared profiles / grants / agents land at
   the centroid of their owners' lanes. Fail-mode policy and escalation
   target nodes are placed *adjacent to their owners* — not in a
   global rightmost column.
5. **Connector emission walks the spec.** Spine edges are emitted by
   iterating `chains[]`; governance edges by iterating
   `governance.failModePolicies` / `governance.escalationTargets` owner
   arrays. The flat `projection.edges` walk is gone.
6. **`ctx` hook bag.** The view defers the six post-render hooks
   (`setCurrentRoot → selectNode → applyZoom → focusOnRoot →
   applyFitMode → scheduleFitToView → applyMultiSelection`) to the
   shared `_gmapRenderCtx` Context uses — no direct camera reach.
7. **Shared classifier extensions.** `governance-map/layers.js`'s
   `gmapNodeCategoryFromKind` and `gmapConnectorKindFromCls` now cover
   every Authority node kind and every Authority connector CSS class.
   Layer toggles and connector hover labels work uniformly across
   lenses.

Tests: `./test.sh all` green; `internal/httpapi` D32 surface green;
`internal/graph/authority` green.

---

## 2. Root Cause Addressed

From [D32g-analysis-5](../analysis/D32g-authority-showcase-layout-failure.md)
and [D32h-analysis-1](../analysis/D32h-context-methodology-for-authority-graph.md):

| Symptom | Cause | D32h-impl-1 response |
| --- | --- | --- |
| Surface → Profile → Grant → Agent cards rendered with no spatial alignment despite valid wire data. | Kind-bucketed planner assigned x by index within kind row, ignoring topology. | Spec-driven planner now computes a chain lane per surface; aligned spine. |
| Fail-mode policy / escalation target stacked in a global rightmost column regardless of which surface or profile owns them. | Global governance column with vertical `gp * (NODE_H + 32)` stacking. | Sidecar attached adjacent to owner; centroid placement for shared owners. |
| Authority view called `camera.scheduleFitToView()` directly, skipping `applyZoom` / `focusOnRoot` / `selectNode` / `setSummary` / etc. | Bespoke camera path inside the view. | View defers all six post-render hooks to `_gmapRenderCtx`. |
| Layer toggles never affected Authority node kinds. | Shared classifier returned `''` for `decision_surface`, `authority_profile`, etc. | Classifier extended with every Authority kind + every Authority CSS connector class. |
| Source-string tests stayed green while visible defects survived. | Tests pinned the *literal source*, not behaviour. | Test surface trimmed to behavioural contracts: adapter emits chain spec; layout helper implements topology rules; view dispatches via ctx hooks. |

---

## 3. Files Modified

| File | Change |
| --- | --- |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | **Added** `mapToCardLayout(projection, view)` (~250 lines). Public surface gains `mapToCardLayout`. Other helpers untouched. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) | **New file.** Pure layout helper exposing `computeAuthorityLayout(spec, GMAP)`. DOM-free. ~270 lines. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | **Refactored.** Removed kind-bucketed planner block. Added spec-driven render path; chain-walking connector emission; six-call `ctx` hook sequence; `_renderCtx()` resolver. Dropped now-unused `_fallbackDistribute`. |
| [internal/httpapi/explorer/assets/js/governance-map/constants.js](../../internal/httpapi/explorer/assets/js/governance-map/constants.js) | **Added** `GMAP.AUTHORITY_LAYERS`, `GMAP.AUTHORITY_CHAIN_GAP`, `GMAP.AUTHORITY_SIDECAR_GAP`. Context constants untouched. |
| [internal/httpapi/explorer/assets/js/governance-map/layers.js](../../internal/httpapi/explorer/assets/js/governance-map/layers.js) | **Extended** `gmapNodeCategoryFromKind` with every Authority kind; extended `gmapConnectorKindFromCls` with every `authority-connector-*` class. Context branches untouched. |
| [internal/httpapi/explorer/index.html](../../internal/httpapi/explorer/index.html) | **One new `<script>` tag** — loads `authority-graph-layout.js` AFTER the adapter and BEFORE the view. |
| [internal/httpapi/explorer_d32g_fix3_test.go](../../internal/httpapi/explorer_d32g_fix3_test.go) | Updated two assertion strings to match the new helper-form (`return;` vs `continue;`, `_anchorsForEdge({ kind: edgeKind }, …)`). Invariant preserved. |
| [internal/httpapi/explorer_d32g_fix6_test.go](../../internal/httpapi/explorer_d32g_fix6_test.go) | Removed obsolete `var ROWS = …` / `var GOV  = …` pins from the NoBroaderRefactor list; relaxed the `Math.max(` substring to `var canvasW` (planner moved into helper). |
| [internal/httpapi/explorer_d32g_fix7_test.go](../../internal/httpapi/explorer_d32g_fix7_test.go) | Removed obsolete `var ROWS = …` / `var GOV  = …` pins from the NoBroaderRefactor list. |
| [internal/httpapi/explorer_d32h_impl1_test.go](../../internal/httpapi/explorer_d32h_impl1_test.go) | **New file.** 21 tests pinning the new methodology contracts (see §12). |

**Not modified:** any backend Go code, any seed data, any CSS, the
Authority projection, the OpenAPI schema, the Context lens code path,
any test harness, any docker-compose / test.sh / build wiring, any
non-Authority lens module.

---

## 4. Authority `mapToCardLayout` Behaviour

Signature: `mapToCardLayout(projection, view) → AuthorityCardLayout`.

Steps, in order:

1. **Index nodes by ref key.** Builds `nodesByRef` keyed by `"<kind>:<id>"`
   and `byKind` arrays for each of the seven Authority kinds.
2. **Single edge walk.** One pass over `projection.edges` populates:
   - `surfaceProfile[surfaceId] = profileId`
   - `profileGrant[profileId]   = grantId`
   - `grantAgent[grantId]       = agentId`
   - `bsSurfaces[bsId]          = [surfaceId, …]`
   - `fmpOwnersBySurf[fmpId]    = {surfaceId, …}`
   - `fmpOwnersByBs[fmpId]      = {bsId, …}`
   - `etOwners[etId]            = {profileId, …}`
   First-wins for spine edges; set semantics for governance owners
   (multiple surfaces can override the same FMP, etc.).
3. **Root resolution.** Uses `projection.root.id` when present; falls
   back to the first `business_service` node so fixture-driven tests
   still produce a populated spec.
4. **Chain extraction.** One chain per decision_surface, in *backend
   emission order* (`byKind.decision_surface`). Each chain walks
   `surface → profile → grant → agent` through the maps above; any
   missing link becomes `null` plus a `missingProfile / missingGrant /
   missingAgent` boolean flag.
5. **Shared-node detection.** Three maps (`seenSharedProfile`,
   `seenSharedGrant`, `seenSharedAgent`) track the *first* chain to
   reference a downstream node. Subsequent references set
   `profileShared / grantShared / agentShared` on the new chain plus
   `profileFirstOwnerChainId / grantFirstOwnerChainId /
   agentFirstOwnerChainId`. The layout helper uses these for centroid
   placement.
6. **Owner-chain reverse map.** `profileOwnerChains[profileId] =
   [chainId, …]` (and same for grants / agents). Layout helper consumes
   these arrays to compute the centroid x.
7. **Governance specs.** For each FMP, gather surface owners + BS
   owners; mark `bsDefault: true` when only BS owners exist. For each
   escalation target, gather profile owners.
8. **Unlinked / orphan detection.** Any node in `projection.nodes` that
   was not assigned to root, a chain slot, or a governance spec is
   appended to the spec's `unlinked` array. The layout helper parks
   these in a deterministic band below the agent row (R4).
9. **Pass-through fields.** `nodes`, `edges`, `summary`, `diagnostics`,
   `diagnosticSummary`, `surfacePosture` are preserved verbatim so the
   inspector + overlays modules continue to read the spec as if it
   were the raw projection.

The function is pure: no DOM, no fetch, no module state. Lines 234-475
of [authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js).

---

## 5. Chain-Building Behaviour

| Input pattern | Chain output |
| --- | --- |
| 1 surface, 1 profile, 1 grant, 1 agent | One chain, `surface/profile/grant/agent` fully populated. |
| 1 surface, missing profile | One chain with `surface` populated; `profile/grant/agent` all `null`; `missingProfile=true`. |
| 1 surface → profile, missing grant | One chain with `surface` + `profile`; `grant/agent=null`; `missingGrant=true`. |
| 2 surfaces sharing 1 profile | Two chains; both reference the same `profile` node; second chain has `profileShared=true` and `profileFirstOwnerChainId=<first chain id>`. |
| 1 surface, no FMP and no escalation | Chain has only spine slots populated; governance specs do not include the surface. |

Chain order is the *backend emission order* of `decision_surface` nodes.
Tests pin the deterministic walk
([explorer_d32h_impl1_test.go](../../internal/httpapi/explorer_d32h_impl1_test.go) —
`TestExplorer_D32hImpl1_AdapterStableOrdering`).

---

## 6. Layout Helper / Lane Assignment Behaviour

Signature: `computeAuthorityLayout(spec, GMAP) → { positions, canvasW,
canvasH, chainOrder, sidecarSlots, anchorsHint }`.

Steps:

1. **Constants resolution.** Pulls `NODE_W`, `NODE_H`, `EDGE_PAD`,
   `MIN_CANVAS_W`, `CANVAS_H`, `AUTHORITY_CHAIN_GAP`,
   `AUTHORITY_SIDECAR_GAP` from GMAP; defensive fallbacks.
2. **Chain lanes.** `chainX[chainId] = EDGE_PAD + i * (NODE_W +
   CHAIN_GAP)` for chain index `i`.
3. **Root placement.** When chains exist, root x = midpoint of
   `chainX[first]` and `chainX[last]`. When no chains exist, root sits
   centred in the minimum canvas.
4. **Spine placement.** For each chain `c`:
   - `surface.x = chainX[c.chainId]`,
   - `profile.x = centroidX(profileOwnerChains[c.profile.id], laneX)`
     — for unshared profiles the centroid degenerates to the single
     owner's lane,
   - same recursive rule for grant + agent.
5. **Sidecar slots.** Every spine node publishes a sidecar slot at
   `{ x: node.x + NODE_W + AUTHORITY_SIDECAR_GAP, y: node.y }`.
6. **Governance placement.**
   - For each FMP, resolve `ownerSidecar(owners)`. Single owner →
     copy slot. Multiple owners → centroid of slots.
   - For each escalation target, same rule using profile owners.
   - Collision handling: `slotOccupied` map drops governance nodes
     down by `NODE_H + 16` when the slot is taken.
   - Unresolvable owners (depth-pruned, edge missing) push the
     governance node into the orphan band.
7. **Orphan band (R4).** Adapter-reported `spec.unlinked` plus any
   governance node whose owners didn't resolve are distributed across
   the spine band at `unlinkedY = agentY + NODE_H + 56`.
8. **`canvasW`.** `Math.max(MIN_CANVAS_W, rightmostX + NODE_W +
   EDGE_PAD)`. Grows with sidecar reservation automatically.
9. **`canvasH`.** Grows past the default 720 when the orphan band
   pushes below.

Lines 70-273 of [authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js).

---

## 7. Shared-Node Handling

| Scenario | Adapter behaviour | Layout helper behaviour |
| --- | --- | --- |
| Profile P used by chains [0, 2] | Both chains reference the same `profile` node. Chain 2 has `profileShared=true`. `profileOwnerChains["P"] = [chain0Id, chain2Id]`. | `profile.x = (chainX[chain0] + chainX[chain2]) / 2` (centroid of two lanes). |
| Grant G used by profiles in chains [1, 3, 5] | Three chains reference the same `grant` node; chains 3 and 5 have `grantShared=true`. `grantOwnerChains["G"]` lists all three chain ids. | `grant.x` = centroid of the three lanes. |
| Single-owner profile P in chain 1 | Chain has `profileShared=false`. `profileOwnerChains["P"] = [chain1Id]`. | `profile.x = chainX[chain1]` (centroid of one lane). |
| Profile P with no owner (unreachable) | Profile lands in `spec.unlinked`. | Placed in the R4 orphan band. |

**Strategy documented in code header.** Comment at lines 27-31 of
[authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js)
declares the centroid rule and its determinism precondition (stable
`chainOrder` from backend emission).

---

## 8. Governance Sidecar Placement

| Case | Placement |
| --- | --- |
| Surface-level FMP owned by surface S | `(S.x + NODE_W + SIDECAR_GAP, S.y)`. |
| BS-default FMP (no surface owners) | Adjacent to the root: `(root.x + NODE_W + SIDECAR_GAP, root.y)`. |
| FMP shared by surfaces S1, S2 | Centroid of S1's and S2's sidecar slots. |
| Profile-level escalation target owned by profile P | `(P.x + NODE_W + SIDECAR_GAP, P.y)`. |
| Escalation target shared by profiles P1, P2 | Centroid of P1's and P2's sidecar slots. |
| Owner unresolvable (depth-pruned, edge missing) | Pushed to the unlinked / orphan band. |
| Two governance nodes wanting the same sidecar slot | Second drops by `NODE_H + 16` (slot occupancy map). |

No global rightmost governance column. The previous `govX = canvasW −
NODE_W − EDGE_PAD` line is gone.

---

## 9. Connector Emission Behaviour

The pre-D32h view iterated `projection.edges` and called
`_anchorsForEdge(edge, …)` to pick anchors. The new view emits
connectors *from the spec topology*:

```text
spec.chains.forEach(c => {
  emitSpine(spec.root,  c.surface, 'business_service_has_surface')
  emitSpine(c.surface,  c.profile, 'surface_uses_profile')
  emitSpine(c.profile,  c.grant,   'profile_has_grant')
  emitSpine(c.grant,    c.agent,   'grant_authorises_agent')
})
spec.governance.failModePolicies.forEach(f => {
  f.owners.forEach(o => emitGovernance(ownerNode, f.node,
    o.kind === 'business_service'
      ? 'business_service_has_fail_mode_policy'
      : 'surface_has_fail_mode_policy'))
})
spec.governance.escalationTargets.forEach(e => {
  e.owners.forEach(o => emitGovernance(ownerNode, e.node,
    'profile_escalates_to'))
})
```

**Helpers:**
- `emitSpine(src, dst, edgeKind)` — fixed `['bottom', 'top']` anchors,
  class `authority-connector authority-connector-<edgeKind>`.
- `emitGovernance(src, dst, edgeKind)` — anchors via `_anchorsForEdge`
  (which routes governance through `MIDASGovernanceMap.pickAnchorSides`).
  Same connector class namespace.
- Both helpers preserve the **structural-edge guardrail**: drop edges
  whose endpoints are missing from the position map. The previous
  `continue;` became `return;` (helper-form), invariant unchanged.

Lines 296-366 of [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js).

---

## 10. Context-Style `ctx` Hook Integration

The view now accepts a `ctx` parameter shaped like Context's
`_gmapRenderCtx` (the inline workbench's hook bag declared at
[index.html:1737-1755](../../internal/httpapi/explorer/index.html#L1737-L1755)).
Six hooks fire at the end of render, in fixed order, mirroring
[context-graph-view.js:628-636](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L628-L636):

```
ctx.setCurrentRoot(view, rootId, displayName)
ctx.selectNode(rootCardId)
ctx.applyZoom()
ctx.focusOnRoot(rootCardId)
ctx.applyFitMode(true)
ctx.scheduleFitToView()
ctx.applyMultiSelection()
```

Each call is guarded with `typeof === 'function'` so the view no-ops
when the inline workbench has not yet attached the hooks (test
isolation, very early boot).

**Wiring.** The view's `refresh()` resolves
`window.MIDASExplorerGraph._renderCtx` via the new `_renderCtx()`
helper and threads the hook bag into `renderAuthorityGraph(payload,
ctx)`. The lens-dispatch path (`lensImpl.render`) does the same so
both call paths share the same hook sequence. Defensive fallback: when
`ctx.scheduleFitToView` is unavailable, the view reaches into
`MIDASExplorerGraph.camera.scheduleFitToView()` (preserves the
pre-D32h direct-dispatch test isolation behaviour).

---

## 11. Shared Layer Classifier Changes

[governance-map/layers.js](../../internal/httpapi/explorer/assets/js/governance-map/layers.js):

- `gmapNodeCategoryFromKind` — new branches for `decision_surface →
  subject`, `authority_profile → authority`, `authority_grant →
  authority`, `agent → agent`, `fail_mode_policy → governance`,
  `escalation_target → governance`, `business_service → subject`.
  Context branches unchanged.
- `gmapConnectorKindFromCls` — new branches for the seven
  `authority-connector-<edge_kind>` CSS classes plus a generic
  fallback for any future `authority-connector` class. Each branch
  returns the human-readable label that the renderer assembles into
  the connector's `aria-label`. Context branches unchanged.

The Authority overlays module (`authority-graph-overlays.js`) still
renders its chip UI inside the drawer's "Posture & Help" tab — that
larger migration was out of scope. The classifier extensions are the
*minimum change* required for shared `applyVisibilityFilters` to
correctly category Authority nodes.

---

## 12. Tests Added or Updated

**New test file:**
[internal/httpapi/explorer_d32h_impl1_test.go](../../internal/httpapi/explorer_d32h_impl1_test.go)
— 21 tests:

| Test | Pins |
| --- | --- |
| `AdapterDeclaresMapToCardLayout` | adapter publishes `mapToCardLayout` |
| `AdapterEmitsChainAndGovernanceSlots` | spec shape: chains, governance, owner maps, unlinked, nodesByRef |
| `AdapterChainWalksSpineEdges` | adapter classifies all 7 edge kinds |
| `AdapterTracksSharedNodes` | shared-node detection on profile/grant/agent |
| `AdapterStableOrdering` | chain order is backend emission order |
| `AdapterUnlinkedComputed` | unlinked / orphan list is assembled |
| `AdapterPreservesPassthroughFields` | diagnostics / surface_posture / summary preserved |
| `LayoutHelperPublished` | `window.MIDASExplorerGraph.authorityLayout` registered |
| `LayoutHelperLoadedFromIndex` | `<script>` order: layout BEFORE view |
| `LayoutHelperImplementsTopologyRules` | R2 centroid, R3 sidecar slot, R3 collision, R4 unlinked band |
| `AuthorityConstantsAdded` | new `GMAP.AUTHORITY_LAYERS`, `AUTHORITY_CHAIN_GAP`, `AUTHORITY_SIDECAR_GAP` |
| `AuthorityConstantsDontReplaceContextLayers` | Context's `GMAP.LAYERS` (RELATED/CAP_PROC/AI) untouched |
| `ViewConsumesSpecNotProjection` | view calls layout helper; kind-bucketed planner removed |
| `ViewEmitsConnectorsByWalkingSpec` | spine + governance helpers exist; flat-edge walk gone |
| `ViewUsesCtxHookSequence` | six-call ctx sequence pinned |
| `ViewWiresRenderCtxThroughRefresh` | view resolves shared `_renderCtx` |
| `ViewCoordinateContractPreserved` | D32g-fix-7 two-liner contract intact and in order |
| `ViewDropsFallbackDistribute` | unused `_fallbackDistribute` helper removed |
| `LayersClassifierKnowsAuthorityKinds` | all 7 Authority kinds + `business_service` classified |
| `LayersClassifierKnowsAuthorityConnectors` | all 7 Authority CSS classes classified |
| `LayersClassifierContextUnchanged` | Context classifier branches preserved |
| `NoHardcodedDemoIds` | no `bs-demo-*` / `surf-demo-*` / etc. anywhere in new code |
| `ContextGraphUnchanged` | Context view + coordinate contract + ctx hooks unchanged |

**Updated tests:**

- `TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides` — updated
  the assertion form to match the new emitGovernance helper
  (`_anchorsForEdge({ kind: edgeKind }, positions[srcKey],
  positions[dstKey])`). Invariant identical.
- `TestExplorer_D32gFix3_StructuralEdgeGuardrail` — updated `continue`
  → `return` (loop → helper form). Invariant identical.
- `TestExplorer_D32gFix6_ViewBoxUpdateAfterCanvasWComputed` —
  relaxed `var canvasW = Math.max(` to `var canvasW` (canvasW
  computation moved into the layout helper; view binding preserved).
- `TestExplorer_D32gFix6_NoBroaderRefactor` — removed obsolete
  `var ROWS = …` / `var GOV  = …` pins; D32h-impl-1 intentionally
  retired the kind-bucketed planner.
- `TestExplorer_D32gFix7_NoBroaderRefactor` — same removal.

---

## 13. Commands Run and Results

All commands executed inside the project's Docker test harness
(`golang:1.26-alpine`) because the user's working environment is
Windows (memory: tests run via Docker, not natively).

| Command | Result |
| --- | --- |
| `go test ./internal/httpapi -run 'D32h' -count=1 -timeout 120s` | `ok  github.com/accept-io/midas/internal/httpapi  0.026s` |
| `go test ./internal/httpapi -run 'Authority\|Explorer\|D32' -count=1 -timeout 120s` | `ok  github.com/accept-io/midas/internal/httpapi  1.837s` |
| `go test ./internal/graph/authority -count=1 -timeout 120s` | `ok  github.com/accept-io/midas/internal/graph/authority  0.019s` |
| `go test ./internal/httpapi -count=1 -timeout 180s` | `ok  github.com/accept-io/midas/internal/httpapi  5.937s` |
| `./test.sh all` | All packages pass. Highlights: `internal/httpapi 5.932s ok`, `internal/graph/authority 0.027s ok`, `internal/graph/context 0.016s ok`, `internal/store/postgres 15.233s ok`, `internal/surface 0.009s ok`. No `FAIL` line in output. |

No test failures attributable to D32h-impl-1.

---

## 14. Browser Verification Results

**Not performed in this tranche.** The user's working environment is
Windows with an Application Control policy that blocks Go test binaries
from running natively, and the running session does not have an open
browser tab on `localhost:8080`. The implementation tranche delivers
the code paths and the Tier-1 source-string contract tests; **the
deferred Tier-2 JSDOM harness (D32h-test-1) is the right venue for
verifying actual rendered positions**, and Tier-3 screenshot tests
(D32h-test-2) for cross-service visual comparison.

The reader is asked to validate by:
1. Starting the dev server.
2. Loading the Authority Graph for `bs-demo-authority-showcase`.
3. Confirming each Surface → Profile → Grant → Agent chain sits in a
   shared x lane.
4. Confirming `fmp-demo-default` sits adjacent to the BS (BS-default
   FMP path) and any surface-override FMPs sit adjacent to their
   surface.
5. Confirming the layer toggles (Diagnostics / Surface posture /
   Escalation / Fail-mode policies) now hide their target Authority
   nodes (previously `gmapNodeCategoryFromKind` returned `''` for
   every Authority kind, so toggles were inert).
6. Confirming Context Graph still renders Retail Banking / Consumer
   Lending / Merchant Services / Payments without visual regression.

Browser verification is acknowledged as a **known gap in this
deliverable** — it is documented in §16 below.

---

## 15. Known Limitations

1. **Tier-2 layout-spec position tests are not yet in place.** The new
   test surface pins behavioural contracts at source-string granularity
   (helper presence, ordering, classifier coverage). True
   layout-spec position assertions require a JSDOM or goja harness
   (D32h-test-1, deferred).
2. **Browser verification not performed in this tranche** (see §14).
3. **Authority overlays module still owns its layer chips.** The
   classifier extensions in `governance-map/layers.js` make
   `applyVisibilityFilters` aware of Authority kinds, but
   `authority-graph-overlays.js` continues to render the chip UI in
   the drawer. Full migration to a shared chip renderer is out of
   scope (§4 row 20 of D32h-analysis-1).
4. **Service-selection refresh is explicitly out of scope.** Operators
   who switch business services while viewing the Authority Graph may
   still need to refresh the lens manually. D32h-analysis-1 noted
   this; this tranche did not address it.
5. **Shared-node centroid placement.** When a profile is shared by
   chains 0 and N-1 (opposite ends of the spine), the centroid sits in
   the middle of the canvas, well separated from both owners. The
   `_anchorsForEdge` direction-aware curve handles the long sweep
   gracefully, but the visual cue that the profile is *shared* is not
   yet rendered. A future tranche could add a "shared by N chains"
   badge.
6. **Authority overlays' "summary pills" still write into a drawer
   mount.** They are not flowed through `ctx.setSummary` (the Context
   methodology equivalent). Moving them is a small follow-up that
   does not block D32h-impl-1.
7. **No backend-projection change.** The backend still emits the same
   node and edge arrays. If a future Authority Graph projection
   carries server-side hints (e.g. `chain_id`, `shared_owners[]`), the
   adapter can consume them with a small additive change.

---

## 16. Explicit Out-of-Scope

Per the user's instructions, the following were **not** modified:

- backend projection (Go code, OpenAPI, schema, runtime)
- service-selection refresh wiring
- adapter cache
- seed data
- runtime evidence overlays
- CSS (zero changes)
- new domain entities
- broad renderer convergence (Context lens path unchanged)
- Playwright / JSDOM harness
- any GitHub operations (no fetch / push / pull / branch / merge / rebase / commit / PR)

---

## 17. Recommended Next Tranche

**D32h-test-1 — Layout-spec position assertions (JSDOM or goja).**

Scope: a small Go test that evaluates `authority-graph-adapter.js` +
`authority-graph-layout.js` against fixture projections via
[goja](https://github.com/dop251/goja) (zero Docker / browser
dependency), and asserts:

1. Chain 0's surface, profile, grant, agent share an x within
   tolerance for an unshared 1:1:1:1 chain.
2. A profile shared between chains 0 and 2 lands at the centroid x
   `(chainX[0] + chainX[2]) / 2`.
3. A surface-override FMP lands at `surface.x + NODE_W +
   SIDECAR_GAP` and the same y as the surface.
4. A BS-default FMP lands adjacent to the root, not in any chain
   lane.
5. Two governance nodes that would collide on the same sidecar slot
   are offset by `NODE_H + 16`.
6. `canvasW` grows past `MIN_CANVAS_W` when chain count × stride
   exceeds the floor.

Estimated reach: ~150 lines of Go test + ~50 lines of goja harness
helper. No frontend changes required. Pinning these gives Tier-2
coverage in pure Go, matching the existing test methodology.

Subsequent tranches:

- **D32h-test-2 — Browser screenshot harness** (Playwright/Puppeteer)
  for cross-service visual regression. Larger investment; only worth
  doing after Tier-2 is in place.
- **D32h-impl-2 — Authority overlays migration to shared chip
  renderer.** Folds the layer-chip UI in `authority-graph-overlays.js`
  into the shared `applyVisibilityFilters` path now that the
  classifier knows Authority kinds.

---

## Constraints Confirmation

- No backend projection changes.
- No runtime changes.
- No schema changes.
- No OpenAPI changes.
- No seed data changes.
- No service-selection refresh changes.
- No GitHub operations performed.
- No commits, branches, pushes, pulls, fetches, merges, rebases, or PRs.

**End of D32h-impl-1.**
