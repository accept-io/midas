# Authority Graph Design Specification (Comprehensive)

## 1. Purpose

The Authority Graph is a graph lens within the MIDAS Explorer. Its
purpose is to make delegated authority visible, inspectable, and
governable.

It answers five primary questions:

1. Who has authority over this decision surface?
2. What authority has been granted?
3. Which agent is authorised?
4. What happens when authority, policy, escalation, or fail-mode
   handling breaks?
5. Where are the governance gaps?

The Authority Graph does not show every governance detail as a
first-class node by default. Detailed governance interpretation
belongs in the contextual letterboxes.

## 2. Design heritage and references

This design draws on established graph-visualisation conventions:

- **Neo4j Bloom / Browser** for node-category colour coding,
  relationship typing, and the principle that node shape and colour
  carry semantic meaning before labels are read.
- **Palantir Foundry / Gotham** for the topology-owned layout
  pattern (one lane per primary entity), the three-zone shell
  (canvas / right drawer / bottom workbench), and selection-driven
  detail panels.
- **The existing MIDAS Context Graph** for connector geometry,
  camera contract, layer model, token palette, and selection hooks.

Where this specification omits a detail, follow Context Graph
conventions. The constraints already encode most visual decisions;
this document specifies only the Authority-specific delta.

## 3. Product model

The Explorer graph workbench is a reusable shell. It is not owned by
Context Graph and it is not owned by Authority Graph.

| Shared shell area | Responsibility |
|---|---|
| Main canvas | Graph topology rendering |
| Right letterbox / drawer | Inspector, diagnostics, posture/help |
| Bottom letterbox / workbench | Lens-specific operational detail |
| Camera controls | Zoom, fit, focus, pan |
| Selection plumbing | Common dispatch and hook model |
| Layer controls | Visibility and overlay controls |
| Lens dispatch | Determines which lens supplies content |

The shell owns the spaces. The active lens owns the content.

Authority Graph must not create a separate shell, route, camera
model, drawer framework, or bottom workbench framework.

| Lens | Canvas | Right letterbox | Bottom letterbox |
|---|---|---|---|
| Context | Context topology | Context inspector | Drift Analytics |
| Authority | Authority topology | Authority inspector | Authority Workbench |
| Future Knowledge | Policy topology | Knowledge inspector | Knowledge workbench |

## 4. Visual language

### 4.1 Node category colour coding

Each node kind has a stable category colour that carries semantic
meaning. Colours come from `tokens.css`; both dark and light themes
must work.

| Node kind | Category | Token (border / accent) | Shape |
|---|---|---|---|
| business_service | Anchor | `--primary` | Rounded rectangle, slightly larger |
| decision_surface | Decision point | `--badge-info` | Rounded rectangle |
| authority_profile | Authority config | `--badge-good` (active) / `--slate-400` (inactive) | Rounded rectangle |
| authority_grant | Authorisation link | `--badge-warn` | Rounded rectangle, narrower |
| agent | Actor | `--on-surface` (active) / `--badge-bad` (blocked/suspended) | Rounded rectangle with status dot |
| fail_mode_policy | Governance fallback | `--slate-400` | Rounded rectangle, dashed border to indicate sidecar |
| escalation_target | Governance route | `--slate-400` | Rounded rectangle, dashed border to indicate sidecar |

Card background is `--surface-container-low` for all kinds. The
category colour applies to the left border (4px), the kind label
strip at the top of the card, and the selection ring when active.

Status overrides category colour for agents specifically:
`operational_state === 'blocked' | 'suspended'` flips the agent
border to `--badge-bad` and adds a red status dot in the top-right.

### 4.2 Connector line styling

Connectors are coloured by edge kind. All connectors use the
existing `pickAnchorSides` routing and curve style from Context.

| Edge kind | Stroke token | Style | Width |
|---|---|---|---|
| business_service_has_surface | `--primary` | Solid | 1.5px |
| surface_uses_profile | `--badge-info` | Solid | 1.5px |
| profile_has_grant | `--badge-warn` | Solid | 1.5px |
| grant_authorises_agent | `--badge-good` | Solid | 1.5px |
| surface_has_fail_mode_policy | `--slate-400` | Dashed (4,2) | 1px |
| business_service_has_fail_mode_policy | `--slate-400` | Dashed (4,2) | 1px |
| profile_escalates_to | `--slate-400` | Dashed (4,2) | 1px |

Spine edges (the first four) are solid. Governance sidecar edges
(the last three) are dashed to signal "secondary relationship."

Selected node's incoming and outgoing connectors get +0.5px width
and full opacity. Non-selected connectors at 0.7 opacity when any
node is selected.

### 4.3 Typography

All text uses `--font-display` for consistency with Context. Sizes
follow Context's existing card typography.

### 4.4 Selection

Selected node: 2px ring in `--primary`, full opacity. Other nodes at
0.6 opacity. Selected node's chain (its surface, profile, grant,
agent — whichever applies) stays at full opacity to preserve chain
context.

### 4.5 Hover

Hover: 1px ring in `--outline-variant`. Matches Context.

## 5. Canvas layout

### 5.1 Core principle

The graph is **topology-owned, not kind-owned**.

Wrong model: all surfaces in one row, all profiles in another, etc.

Correct model: one authority lane per decision surface, with
profile, grant, and agent stacked vertically in that lane.

### 5.2 Layout constants

These extend the existing `governance-map/constants.js`. Where a
constant already exists in Context, reuse it; where Authority needs
its own, add it.

| Constant | Value | Source |
|---|---|---|
| `NODE_W` | Existing Context value | `constants.js` (reuse) |
| `NODE_H` | Existing Context value | `constants.js` (reuse) |
| `CHAIN_GAP` | Existing Context value | `constants.js` (reuse) |
| `MIN_CANVAS_W` | Existing Context value | `constants.js` (reuse) |
| `AUTHORITY_LANE_GAP` | `CHAIN_GAP` | Horizontal spacing between authority lanes equals Context's chain gap for visual consistency |
| `AUTHORITY_VERTICAL_STEP` | `NODE_H + 40` | Vertical distance between authority levels (surface→profile→grant→agent) |
| `AUTHORITY_SIDECAR_GAP` | Existing if present, else `NODE_W / 2` | Horizontal offset from owner to sidecar node |
| `AUTHORITY_TOP_MARGIN` | 40 | Padding above business service |
| `AUTHORITY_BOTTOM_MARGIN` | 60 | Padding below deepest authority level |

All values are pixels. Layout helper produces positions in these
units; the camera and viewBox contract is unchanged from Context.

### 5.3 Primary authority spine

Business Service at top centre.

Each Decision Surface anchors a vertical authority lane:
Business Service
                      │
    ┌─────────────────┼─────────────────┐
    │                 │                 │
Surface A         Surface B         Surface C
    │                 │                 │
Profile A         Profile B        (No profile badge on
    │                 │             Surface C)
Grant A          (No grant badge
    │            on Profile B)
Agent A

Surfaces distribute horizontally. Each surface's lane runs
vertically: surface → profile → grant → agent.

### 5.4 Lane assignment and spacing

Lanes are assigned in deterministic order: sort surfaces by
`(business_service_id, surface_id)` ascending. This produces stable
output across renders.

Lane X coordinates: equal spacing across the canvas width. If three
lanes, they occupy `chainX[0]`, `chainX[1]`, `chainX[2]` such that
`chainX[i+1] - chainX[i] === NODE_W + AUTHORITY_LANE_GAP`.

Business Service centres horizontally above the lanes:
`bs.x === (chainX[0] + chainX[last]) / 2`.

### 5.5 Vertical alignment

Within a lane, nodes align on x. The vertical y for each level is
deterministic:
bs.y       = AUTHORITY_TOP_MARGIN
surface.y  = bs.y + AUTHORITY_VERTICAL_STEP
profile.y  = surface.y + AUTHORITY_VERTICAL_STEP
grant.y    = profile.y + AUTHORITY_VERTICAL_STEP
agent.y    = grant.y + AUTHORITY_VERTICAL_STEP

This guarantees: same-level nodes line up horizontally; chains line
up vertically; no node overlaps another by construction.

### 5.6 No-overlap guarantee

The layout produces no overlaps on first render by construction:

- Each lane occupies a unique x.
- Within a lane, each level occupies a unique y.
- Sidecar nodes attach at owner.x + NODE_W + `AUTHORITY_SIDECAR_GAP`,
  which is outside the owner's bounding box.
- Centroid placement is preferred only when it does not collide with an occupied node at the same level and remains visually attributable to its owners. If centroid placement collides or exceeds the fallback threshold, use fallback placement.

The base unshared lane layout prevents overlap by construction. Shared nodes and sidecars require explicit collision handling. The layout helper must detect and resolve collisions before returning visibleNodes.

### 5.7 Missing links

Missing links are badges on the upstream card, not layout gaps.

| Scenario | Treatment |
|---|---|
| Surface without profile | Surface card shows "No profile" badge in `--badge-warn` |
| Profile without grant | Profile card shows "No grant" badge in `--badge-warn` |
| Grant without agent | Grant card shows "No active agent" badge in `--badge-warn` |
| Blocked/suspended agent | Agent border flips to `--badge-bad`, status dot top-right |

The lane below the missing link is empty. The next chain level for
that lane is not rendered (no profile means no grant card either —
the chain truncates at the missing link).

### 5.8 Shared nodes

A profile, grant, or agent is shared when multiple chains reference
the same entity. The adapter detects this via `profileOwnerChains`,
`grantOwnerChains`, `agentOwnerChains` reverse maps.

| Shared entity | Layout treatment |
|---|---|
| Shared profile | Place once at centroid of owning surface lanes |
| Shared grant | Place once at centroid of owning profile lanes |
| Shared agent | Place once at centroid of owning grant lanes |

Required badge on the shared node: `"Shared by N"` where N is the
owner count. Badge uses `--badge-info` token, placed below the kind
label strip.

### 5.9 Centroid fallback (browser-evidence-driven)

Centroid placement fails when owners are far apart. The fallback
threshold is determined by browser evidence in D32h-fix-2f, not
hardcoded here.

Provisional rule (to be confirmed by snapshots): if
`|centroid.x - nearest_owner.x| > 1.5 * (NODE_W + AUTHORITY_LANE_GAP)`,
place the shared node at the leftmost owner's lane x instead.

Connector behaviour when shared:

- Each owner draws its own connector to the shared node.
- All connectors share the same edge kind colour.
- If centroid fallback fires (leftmost placement), connectors from
  distant owners are visibly longer; this is correct — the visual
  length signals the sharing distance.

### 5.10 Governance sidecars

Fail-mode policy and escalation target nodes attach to their owners
when their layer is enabled.

| Sidecar | Owner | Position |
|---|---|---|
| BS default fail-mode policy | Business Service | `bs.x + NODE_W + AUTHORITY_SIDECAR_GAP`, same y |
| Surface override fail-mode policy | Decision Surface | `surface.x + NODE_W + AUTHORITY_SIDECAR_GAP`, same y |
| Shared fail-mode policy | Centroid of owners | Same y as owners |
| Escalation target | Authority Profile | `profile.x + NODE_W + AUTHORITY_SIDECAR_GAP`, same y |
| Shared escalation target | Centroid of profiles | Same y as profiles |

Sidecars use dashed borders (per 4.1) to signal "secondary."

Sidecars never stack in a global right column. If two sidecars
would land at the same position (collision), offset the second by
`NODE_H + 16` vertically.

## 6. Layer model

### 6.1 Layers

| Layer | Default | Effect |
|---|---|---|
| Authority spine | On, locked | Shows BS, Surface, Profile, Grant, Agent |
| Diagnostics | On | Shows diagnostic markers/badges |
| Surface posture | On | Shows posture badges (FMP inherited/override/missing/dangling) |
| Fail-mode | Off | Shows fail_mode_policy nodes and FMP edges |
| Escalation | Off | Shows escalation_target nodes and escalation edges |
| Evidence overlay | Off, future | Runtime evidence overlay when implemented |

### 6.2 Layout helper contract
computeAuthorityLayout(spec, GMAP, layerState) → {
positions,
visibleNodes,
visibleEdges,
canvasW,
canvasH,
chainOrder,
sidecarSlots,
anchorsHint
}

The helper is the source of truth for visibility. CSS hiding may
remain as defensive belt-and-braces but does not drive the model.

Hidden optional governance nodes do not affect `canvasW` or
`canvasH`. The helper computes bounds from `visibleNodes` only.

## 7. Connector routing

Connectors use the existing `pickAnchorSides` helper from Context.
No new routing algorithm.

Anchor selection rules for Authority's vertical-lane pattern:

| Source kind | Target kind | Source anchor | Target anchor |
|---|---|---|---|
| business_service | decision_surface | bottom | top |
| decision_surface | authority_profile | bottom | top |
| authority_profile | authority_grant | bottom | top |
| authority_grant | agent | bottom | top |
| business_service | fail_mode_policy | right | left |
| decision_surface | fail_mode_policy | right | left |
| authority_profile | escalation_target | right | left |

These rules feed `pickAnchorSides` as hints; the helper produces
the actual curve geometry consistent with Context.

Connector intersection with node boxes: avoided structurally by
the vertical-lane layout. Spine edges go straight down within a
lane. Sidecar edges go horizontally. No connector should pass
through a node card on first render. If a snapshot shows otherwise
in D32h-fix-2a, the lane spacing constants need adjustment in
D32h-fix-2f.

## 8. Right letterbox: Authority drawer

### 8.1 Tabs

- Inspector
- Diagnostics
- Posture & Help

### 8.2 Inspector tab content

Renders raw selected-node fields. Field set per kind:

| Kind | Fields shown |
|---|---|
| business_service | id, name, owner, service_type, status, fail_mode_policy_id |
| decision_surface | id, version, name, process_id, status, fail_mode_policy_id |
| authority_profile | id, version, name, status, effective_date, confidence_threshold, consequence_threshold, escalation_mode, fail_mode, policy_reference, approved_by, approved_at |
| authority_grant | id, profile_id, agent_id, status, validity_status, effective_date, expires_at, capabilities, constraints |
| agent | id, name, type, owner, model_version, operational_state |
| fail_mode_policy | id, version, name, status, effective_date, origin, managed, business_owner, technical_owner, rule_count_by_class |
| escalation_target | id, name, type, contact_info |

Field labels in `--slate-400`, values in `--on-surface`. Empty
fields omitted, not shown as "—" or "(none)".

### 8.3 Diagnostics tab

Full diagnostic list scoped to the selected node. Each row:
severity badge (`--badge-bad`/`--badge-warn`/`--badge-info`),
diagnostic kind, message.

If no diagnostics, show "No diagnostics for this node."

### 8.4 Posture & Help tab

Layer chips (toggle controls for fail-mode, escalation, diagnostics,
surface posture). Legend (node kinds, edge kinds, badge meanings).
Posture explanation text.

## 9. Bottom letterbox: Authority Workbench

### 9.1 Tabs

- Overview
- Fail Mode
- Escalation
- Grants
- Evidence

### 9.2 Selection-driven content

| Selection | Workbench emphasis |
|---|---|
| No selection | Service-level authority overview |
| Business Service | Service authority posture, surface count, fail-mode coverage |
| Decision Surface | Effective profile, grant, agent, fail-mode posture, diagnostics |
| Authority Profile | Escalation, thresholds, linked grants, linked surfaces |
| Authority Grant | Capabilities, constraints, authorised agent |
| Agent | Linked grants and operational state |
| Fail-mode Policy | Policy detail, owner source, rule counts |
| Escalation Target | Linked profile and escalation context |

### 9.3 Overview tab

Service-level rollup. Stats grid:

- Surface count
- Active profiles / inactive profiles
- Active grants / inactive grants
- Grants without agents
- Active agents / blocked agents / suspended agents
- Stop grants (highlighted if non-zero)
- Fail-mode missing / dangling counts
- Diagnostic severity counts (critical / warning / info)

Each stat is a label/value pair. Critical counts use `--badge-bad`,
warning counts use `--badge-warn`, info counts use `--badge-info`.

When a Business Service is selected, Overview shows the same stats
scoped to that service. When a deeper node is selected, Overview
shows a node summary plus the parent BS rollup.

### 9.4 Fail Mode tab

Per-selection emphasis per section 9.2. Fields per BS or surface
selection:

- Effective policy (name + version)
- Source: `business_service_default` | `surface_override` |
  `inherited` | `missing` | `dangling`
- Rule count by correctness class
- Posture: inherited / override / missing / dangling
- Diagnostics for fail-mode

Do not show enforcement state if not present in projection.

### 9.5 Escalation tab

For profile selection:

- Escalation mode (`stop` / `escalate` / `permit_with_evidence`)
- Escalation target name + id
- Confidence threshold
- Consequence threshold
- Dangling/missing escalation diagnostics

For surface selection: surface's profile's escalation info.

### 9.6 Grants tab

For surface/profile/grant/agent selection:

- Active grants table (status, validity, agent, capabilities)
- Capabilities (highlighted: stop, escalate, approve)
- Constraints
- Agent operational state

Stop capability highlighted with `--badge-bad` background tint
when present.

### 9.7 Evidence tab

Projection-backed only.

- Diagnostic count (total)
- Critical / warning / info counts
- Projection metadata if available

Must include exact copy:
`"Runtime evidence overlay is not wired yet for the Authority lens."`

Must not include:
- live evaluation counts
- recent decisions
- runtime rates
- evidence envelope counts
- escalation rates

### 9.8 Empty states

| Tab | No selection | No data |
|---|---|---|
| Overview | "Select a service to see authority posture." | "This service has no surfaces." |
| Fail Mode | "Select a surface or business service." | "No fail-mode policy configured." |
| Escalation | "Select an authority profile or surface." | "No escalation configured." |
| Grants | "Select a profile, grant, surface, or agent." | "No grants linked." |
| Evidence | "Select a node to see evidence summary." | "Runtime evidence overlay is not wired yet for the Authority lens." |

### 9.9 Letterbox chrome

Authority Workbench shares chrome contract with Context's Drift
Analytics tray:

- Collapsed height: 36px (header bar only)
- Expanded height: 320px
- Toggle button with ▲ / ▼ glyph swap
- Smooth `height` transition over 0.18s
- Re-fit camera 200ms after toggle
- Material Design tokens throughout

This is the D32h-fix-2d-converge tranche scope.

## 10. Focus Mode

Focus Mode is the existing `body.gmap-focus-mode` state managed by
Context. Authority Graph inherits Focus Mode behaviour:

- Authority Workbench respects the same default (collapsed).
- Authority canvas uses the same expanded canvas area.
- Layer chips remain accessible via the right drawer.
- Selection behaviour unchanged.

No Authority-specific Focus Mode logic. If Context adds Focus Mode
behaviour later, Authority inherits it.

## 11. Selection model

Single dispatch point: `selectGovernanceMapNode(nodeId)`.
function selectGovernanceMapNode(nodeId) {
gmapSelectedId = nodeId;  // shared selected-node state preserved
const lens = MIDASExplorerStore.getState().selectedGraphLens;
if (lens === 'authority') {
return ExplorerGraph.authorityInspector.selectNode(nodeId);
}
return ExplorerGraph.contextInspector.selectNode(nodeId);
}

The active lens's inspector:
1. Marks the clicked card with `.selected`.
2. Clears `.selected` on other cards.
3. Renders kind-specific content into the drawer Inspector tab.
4. Calls `notifyEvidenceTraySelectionChanged` to refresh the
   workbench.

All graph selection entry points (mouse click, keyboard activation,
ctx hook, render-ctx hook, evidence-tray hook, search-find)
converge through this dispatch.

## 12. Adapter contract
authorityAdapter.mapToCardLayout(payload, view) → {
lens: 'authority',
root,
nodesByRef,
chains,
governance,
diagnostics,
diagnosticSummary,
surfacePosture,
summary,
rawNodes,
rawEdges,
profileOwnerChains,
grantOwnerChains,
agentOwnerChains
}

### 12.1 Chain shape
{
chainId,
surface,
profile,
grant,
agent,
missingProfile,
missingGrant,
missingAgent,
profileShared,
grantShared,
agentShared,
profileFirstOwnerChainId,
grantFirstOwnerChainId,
agentFirstOwnerChainId
}

### 12.2 Governance shape
{
failModePolicies: [{
node,
owners: [{ kind: 'business_service' | 'decision_surface', id }],
shared,
bsDefault
}],
escalationTargets: [{
node,
owners: [{ kind: 'authority_profile', id }],
shared
}]
}

### 12.3 Reverse owner maps

`profileOwnerChains[profileId] → [chainId, chainId, ...]`. Used by
the layout helper for centroid placement and shared-node badges.

## 13. View responsibilities

The Authority view:

1. Reads layer state via `authorityOverlays.getLayerState()`.
2. Calls `computeAuthorityLayout(spec, GMAP, layerState)`.
3. Iterates `visibleNodes` for paint.
4. Iterates `visibleEdges` for connector emission via
   `pickAnchorSides`.
5. Applies coordinate contract
   (`canvas.dataset.baseWidth = canvasW; svg.viewBox = …`).
6. Calls `_paintNode` per visible node (handles category colour,
   badges, posture indicators).
7. Calls overlays module for layer chip updates.
8. Calls workbench module's `render()` for bottom letterbox refresh.
9. Calls ctx hooks (`setCurrentRoot`, `scheduleFitToView`).

The view does NOT:

- Walk raw projection edges.
- Compute layer visibility.
- Own a separate camera path.
- Own a separate drawer framework.
- Own a separate workbench framework.

## 14. Data sources

Allowed:
- `spec.summary`
- `spec.diagnostics`
- `spec.diagnosticSummary`
- `spec.surfacePosture`
- `spec.chains`
- `spec.governance.failModePolicies`
- `spec.governance.escalationTargets`
- `spec.nodes` or `spec.rawNodes`
- `spec.edges` or `spec.rawEdges`
- node typed data
- card dataset `nodeDetails`

Disallowed unless explicitly wired:
- fake runtime counters
- fake recent decisions
- fake evidence envelope counts
- fake live rates
- hardcoded demo IDs
- hardcoded demo service names

## 15. Canvas bounds and safe area

- `canvasW` computed from rightmost visible node extent +
  `NODE_W + EDGE_PAD`, minimum `MIN_CANVAS_W`.
- `canvasH` computed from deepest visible node extent +
  `AUTHORITY_BOTTOM_MARGIN`.
- Hidden governance nodes do not contribute to bounds.
- Bottom letterbox reduces visual space but does not clip canvas
  (canvas scrolls if needed).
- Fit-to-view uses computed visible canvas extent.

## 16. Browser verification

Required across canonical services in fixed order:

1. Authority Graph Showcase (dense)
2. Retail Banking (sparse)
3. bs-consumer-lending (medium realistic)
4. bs-consumer-lending under Context lens (regression check)

These are local verification fixtures, not product dependencies. Implementation must not hardcode these names or IDs. If unavailable, use equivalent dense, sparse, and medium services and record the exact service IDs in the evidence report.

### 16.1 Required visual checks per service

| Check | How to verify |
|---|---|
| Default fail-mode and escalation hidden | No FMP or escalation nodes in snapshot when layers off |
| Authority spine readable | Each chain's surface/profile/grant/agent x within 4px of each other |
| Lanes obvious | `chainX` strictly monotonic across chains |
| No node overlaps | No two visible nodes have overlapping bounding boxes |
| No connectors through node boxes | No connector path intersects a node bounding box |
| Missing-link badges visible | For surfaces with `profileStatus === 'missing'`, "No profile" badge present |
| Shared-node badges visible | For nodes with `profileOwnerChains.length > 1`, "Shared by N" badge present |
| Category colours applied | Each node's left border matches its kind's category token |
| Fail-mode layer on: sidecars attach | Each FMP node's x > owner.x + NODE_W |
| Escalation layer on: sidecars attach | Each escalation node's x > profile.x + NODE_W |
| Workbench visible in Authority lens | `#gmap-authority-workbench` visible |
| Drift Analytics visible in Context lens | `#gmap-evidence-tray` visible, Authority workbench hidden |
| No canvas clipping by bottom letterbox | `meta.nodesClippedByBottomRail.length === 0` |
| Selected-node contrast readable | `selected.subElements[*].color` matches design tokens, not muted |

### 16.2 Context regression

- Context canvas unchanged.
- Drift Analytics in bottom letterbox.
- Authority Workbench hidden.
- Context selection still routes to Context inspector.

## 17. Implementation tranche sequence

| Tranche | Purpose |
|---|---|
| D32h-fix-2b | Selection-path lens-aware dispatch (done) |
| D32h-fix-2c | Layout helper layerState contract (done) |
| D32h-fix-2d | Authority Bottom Workbench Letterbox (done — visibility) |
| D32h-fix-2d-converge | Letterbox CSS and behaviour parity |
| D32h-fix-2e | Authority Graph Structural Layout |
| D32h-fix-2f | Browser-observed layout refinement |
| D32h-fix-2h | FD32h-fix-2f — Authority Graph Visual Semantics |
| D32h-clean-1 | Cleanup of obsolete code |

Cleanup last. Do not remove code until the replacement design is
fully working.

## 18. Testing strategy

### 18.1 Keep
Tests protecting: adapter spec shape, lens-aware selection,
layerState contract, visibleNodes/visibleEdges, coordinate contract,
Context regression, Evidence tab honesty, no demo IDs.

### 18.2 Update
Tests pinning obsolete details: view walking `spec.chains` directly,
view emitting connectors through old helper names, two-argument
`computeAuthorityLayout`, CSS-only graph visibility as primary
contract.

### 18.3 Add
Tests for: workbench DOM and module, lens routing, workbench tabs,
projection-backed data usage, Evidence honesty, selection refresh,
layerState filtering, Context preservation, category colour
application, no-overlap invariant (assertable from spec given
constants).

### 18.4 Browser verification
Required for: readability, clipping, contrast, connector clarity,
sidecar placement, workbench visibility, lens switching.

## 19. Non-goals

- backend / schema / OpenAPI / runtime changes
- seed data changes
- service-selection refresh
- deployment / GitHub workflows
- new route / new graph shell / new camera framework
- runtime evidence overlay (future)
- fake evidence counters

## 20. Design acceptance criteria

The Authority Graph design is complete when:

1. Authority uses the shared graph shell.
2. Default Authority canvas shows the authority spine only.
3. Fail-mode and escalation nodes hidden by default.
4. Optional governance nodes available through layers.
5. Hidden optional governance nodes do not shape default canvas
   bounds.
6. No nodes overlap on first render.
7. No connectors pass through node bounding boxes on first render.
8. Category colour coding applied per section 4.1.
9. Connector line colours applied per section 4.2.
10. Missing links visible as badges.
11. Shared nodes badged with "Shared by N".
12. Right letterbox shows Authority inspector / diagnostics /
    posture / help.
13. Bottom letterbox shows Authority Workbench in Authority lens.
14. Context mode still shows Drift Analytics.
15. Authority selection updates both letterboxes.
16. Workbench content is projection-backed.
17. Evidence tab does not fake runtime evidence.
18. Letterbox chrome matches Context's contract (height transition,
    glyph swap, re-fit).
19. Context Graph not regressed.
20. Tests pass.
21. Browser verification confirms readability across all four
    canonical services.

## 21. Summary

The Authority Graph is a graph lens inside the existing MIDAS
Explorer graph workbench. It is topology-owned (one lane per
decision surface), category-colour-coded (per Neo4j conventions),
and three-zone-structured (canvas / right drawer / bottom workbench,
per Palantir conventions).

The shell owns the spaces. The lens owns the content. The layout
helper owns visibility and placement. The view paints what the
helper says is visible.

Default canvas is the authority spine. Governance details live in
sidecars (when their layer is on), the right drawer (raw fields and
diagnostics), and the bottom workbench (interpreted posture). Shared
nodes are badged. Missing links are badges. Nothing overlaps by
construction.

Browser verification confirms readability. Tests protect contracts,
not failed intermediate implementations.