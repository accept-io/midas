# Strategic Graph Interaction Visual Language

Status: Proposed design standard

Owner: MIDAS graph platform

Applies to: Authority Graph, Context Graph, future graph lenses

Last updated: 2026-05-24

## 1. Purpose

This document defines the interaction behaviour and visual state language for MIDAS strategic graphs. It is a sibling to the [Strategic Graph Connector Taxonomy](strategic-graph-connector-taxonomy.md): the connector taxonomy defines edge semantics and connector configuration; this document defines node, card, and connector interaction state.

Interaction state is a first-class graph concern. A graph object is never simply rendered; at every moment it carries a state that reflects the operator's intent (hovered, selected, focused) and the system's interpretation (active, drifted, warning). The platform owns the controlled state vocabulary and the mechanics that translate state to visual treatment. Graph lenses configure which states apply to which objects.

This standard applies across the Authority Graph, Context Graph, and future graph lenses. A graph lens may map domain-specific semantics onto the controlled vocabulary, but the shared graph platform owns the state names, the mirroring conventions between Cytoscape and the HTML overlay tier, the accessibility rules, and the platform versus lens ownership split.

## 2. Scope

This standard covers:

- Node interaction state (hover, select, focus, related, unrelated).
- HTML card interaction state and mirroring from the underlying graph engine.
- Connector interaction state. Connector semantics are cross-referenced to the [Connector Taxonomy](strategic-graph-connector-taxonomy.md) and not duplicated here.
- Inspector handoff rules.
- Graph control separation (camera versus inspector).
- Visual token rules for interaction state.
- Platform versus graph-lens responsibilities for interaction.
- The state-to-class and state-to-attribute contract that future graph lenses converge on.
- Accessibility rules for interaction state.

This standard does not cover:

- Connector semantics, families, or configuration shape. See the [Connector Taxonomy](strategic-graph-connector-taxonomy.md).
- Node card layout, typography, or per-kind colour. Those belong to a separate node card design standard.
- Graph layout algorithms.
- Inspector panel chrome design.
- Final colour token hex values.
- Backend schema or graph data contracts.
- Cytoscape-specific implementation details beyond the mirroring contract.

## 3. Design principles

- Interaction state is platform property. Graph lenses do not define new state names.
- Hover is informative and transient. Selection is intentional and persistent.
- Selection survives mouseout. Selection survives most graph re-renders.
- Selection drives downstream surfaces. Closing those surfaces does not clear selection.
- The selected state is rendered with the platform primary token, not with a per-lens colour.
- Visual state must not depend on colour alone. Every state carries a non-colour cue.
- Selected and hovered states must not change layout or object size. Visual diff is additive only.
- Cytoscape is the source of truth for the engine layer. The HTML overlay tier mirrors engine state; it does not author it.
- Camera control belongs to the camera toolbar. Selection control belongs to the graph engine. Inspector control belongs to the inspector. The three are separate.
- Multi-select is not the default. Single-select is the default contract; multi-select is a declared lens capability.
- Keyboard interaction is a peer to pointer interaction, not a fallback.

## 4. Interaction state vocabulary

The controlled interaction state vocabulary is the closed set below. Lenses may opt into states; lenses must not introduce new state names.

| State | Applies to | Transient or persistent | Meaning |
|---|---|---|---|
| `default` | nodes, cards, connectors | persistent | Baseline rendering. No active interaction, no selection, no semantic overlay. |
| `hovered` | nodes, cards, connectors | transient | Pointer is over the object, or keyboard navigation has the object under cursor. Cleared on pointer leave or focus change. |
| `selected` | nodes, cards, connectors | persistent | Operator has explicitly chosen the object as the subject of inspection. Persists across mouseout and across pointer interaction with other objects. Source of truth is the engine layer. |
| `focused` | nodes, cards, connectors | persistent | Keyboard focus position. Distinct from `selected`: a user may navigate focus through several objects before selecting one. Rendered as `:focus-visible` only; pointer interaction does not produce a focused state. |
| `selected_hovered` | nodes, cards, connectors | transient overlay on persistent | The selected object also has the pointer over it. Combined state, not a separate state name; rendered as the superposition of `selected` and `hovered`. |
| `connected_to_selected` | nodes, cards, connectors | transient (derived) | Object is a direct neighbour of the selected object, or a connector incident to the selected node. Derived from selection state; cleared automatically when selection clears. |
| `unrelated_to_selected` | nodes, cards, connectors | transient (derived) | Object is neither selected nor connected to a selected object during an active selection. Rendered with reduced emphasis. Cleared automatically when selection clears. |
| `disabled` | nodes, cards, connectors | persistent | Object cannot be interacted with in the current context. Hover, select, and focus are suppressed. |
| `inactive` | nodes, cards, connectors | persistent | Object is rendered but is not part of the operationally active graph (for example, a deprecated profile that remains in view for context). Hover and select are allowed; the object reads as muted. |
| `warning` | nodes, cards, connectors | persistent overlay | Object carries a semantic warning relevant to the active graph lens. May co-exist with `selected`, `hovered`, `focused`. Specific warning meaning is supplied by the lens via tooltip and inspector copy. |
| `drift` | nodes, cards, connectors | persistent overlay | Object is associated with a drift signal under the active graph lens. May co-exist with selection states. Specific drift meaning is supplied by the lens. |
| `evidence_signal` | nodes, cards, connectors | persistent overlay | Object is associated with an active evidence signal (audit, observation, attestation) under the active graph lens. May co-exist with selection states. |

States in the first nine rows are interaction states. The last three (`warning`, `drift`, `evidence_signal`) are semantic overlays that compose with interaction states. The platform renders both layers; lenses configure which semantic overlays apply to which objects.

Combined-state notation is the superposition of two state names (for example, `selected` plus `hovered` reads as `selected_hovered` in copy; in code both classes or attribute tokens are present simultaneously). Lenses must not invent compound state names that bypass this notation.

## 5. Node interaction rules

### 5.1 Hover

- Pointer enter on a node sets `hovered` on the node and on the corresponding HTML card.
- Pointer leave clears `hovered`.
- Hover may reveal a transient tooltip or label. Hover must not open a workbench, drawer, or inspector.
- Hover treatment must be visibly weaker than selection treatment. An operator must never confuse a hovered node for a selected one.

### 5.2 Selection

- Click (or `Enter` when focused) on a node sets `selected` on the node and on the corresponding HTML card.
- Selection is the source of truth for the inspector handoff (section 7).
- Single-select is the default. Selecting a different node clears the previous selection.
- Multi-select is opt-in per lens; lenses that opt in must document the selection model and provide a clear deselection affordance.
- Selecting empty canvas clears selection. Selection clear is a valid state.
- Selection persists across mouseout, across other nodes' hover events, and across graph re-renders unless the selected node is removed from the projection.

### 5.3 Deselection

- Clicking empty canvas deselects.
- Clicking the selected node again is a no-op by default. A lens may opt into click-toggle deselection if its inspector handoff makes that natural; the platform default is no-toggle.
- Pressing `Escape` while focus is inside the graph viewport deselects.
- Closing the inspector does not deselect.
- Camera operations (zoom, fit, pan) do not deselect.

### 5.4 Multi-select (non-default)

Multi-select is a declared lens capability. A lens that supports multi-select must:

- Document the selection model (additive on `Shift`, toggling on `Cmd`, lasso, or other).
- Provide a clear total-count affordance.
- Provide a single deselection affordance.
- Render selected nodes with the same `selected` state per object; no separate "multi-selected" state name exists.

The platform default is single-select. Future lenses that need multi-select are expected to justify it in their design tranche.

## 6. HTML card interaction rules

The HTML card overlay tier is the operator-facing surface for node identity, kind, and inspectable summary. Card interaction must mirror the underlying engine state precisely.

### 6.1 Engine state is the source of truth

- The HTML card mirrors the engine layer. The engine (Cytoscape today) owns hover and selection state for the node.
- The card layer must not author selection state independently. Card-level click handlers may delegate to engine selection but must not maintain a separate selection model.
- Card hover may set a transient `hovered` class on the card directly; the corresponding engine node should also reflect hover so emphasis remains consistent across the two tiers.

### 6.2 Selected state mirroring contract

The platform standard for selected-state mirroring is the Authority POC. The behaviour is described here as the platform contract; future graph lenses converge on this behaviour without copying Authority code.

- A cards-tier sync routine subscribes to engine events `position`, `bounds`, `layoutstop`, `add`, `select`, and `unselect`.
- On each sync, the routine reads `node.selected()` from the engine for each visible node and reflects the result onto the card by adding or removing the `selected` class (or by setting `[data-graph-card-state~="selected"]`; see section 10).
- The routine is idempotent. A node that is selected before and after a sync produces no DOM mutation.
- The routine is rAF-coalesced when sync events fire at high frequency (drag, pan).
- When a node is removed from the projection, its card is removed; no stale selection state may persist on a dangling card.

### 6.3 Card selected visual treatment

- The card selected state must be rendered with the platform primary token. Hardcoded colours are forbidden.
- The selected state must use a non-colour visual cue. The platform standard is an outline ring rendered with `box-shadow` (a 2 px primary ring). The ring is the non-colour cue; the colour reinforces.
- The card must not change size, padding, margin, or layout when entering or leaving the selected state.
- The card must not change z-index or stacking when entering or leaving the selected state, except for the rendering platform's intentional selected-priority layering rule if one is in force.
- The card retains any per-kind border colour established by the node card design. The selected state is additive (the ring is outside the border), not a replacement.

The Authority POC implementation reference is:

```text
.cytoscape-poc-html-card.selected {
  box-shadow:
    0 0 0 2px var(--primary, fallback),
    0 4px 20px <accent halo>;
}
```

The implementation reference uses `var(--primary)`. The fallback colour in the reference is implementation detail, not part of the standard. The standard says: platform primary token, outline ring as the non-colour cue, no layout change.

### 6.4 Hover state and mouseout

- Card hover treatment is weaker than selected treatment. The two states must be visually distinguishable at a glance.
- Mouseout from a hovered card clears `hovered`.
- Mouseout from a selected card does not clear `selected`. The selected state survives mouseout in every case.
- Mouseout never triggers an inspector close or a selection clear.

### 6.5 Disabled cards

- A card in `disabled` state does not respond to pointer or keyboard interaction.
- The card cannot be selected, hovered, or focused.
- The disabled state is rendered with reduced opacity and a non-colour cue (the platform default is reduced opacity plus a strikethrough or "not allowed" cursor; lenses may override the non-colour cue with platform approval).

## 7. Connector interaction rules

Connector semantics, families, and configuration are defined in the [Connector Taxonomy](strategic-graph-connector-taxonomy.md). The interaction rules below apply across all connector families.

### 7.1 Connector hover

- Pointer over a connector sets `hovered` on the connector.
- Hover reveals the connector label (under `labelPolicy: hover` from the connector taxonomy), the source and target, and a short relationship summary.
- Connector hover must not open a workbench or inspector. Hand-off, if any, follows the taxonomy's `handoffTarget` rule.

### 7.2 Connector selection

- Connector selection sets `selected` on the connector.
- A selected connector renders a compact relationship summary, per the connector taxonomy section 7.2.
- Selecting a connector does not clear node selection. The two selection surfaces (node, connector) coexist.
- Selecting empty canvas clears connector selection. `Escape` clears connector selection.

### 7.3 Connected-edge emphasis on node selection

When a node is selected:

- Connectors incident to the selected node receive `connected_to_selected`.
- Other connectors receive `unrelated_to_selected`.
- Nodes incident to those connectors (one-hop neighbours) receive `connected_to_selected`.
- Other nodes receive `unrelated_to_selected`.

These derived states clear automatically when the node selection clears. Lenses do not control derivation; the platform owns it.

The visual treatment is:

- `connected_to_selected` is rendered with default emphasis (no de-emphasis applied).
- `unrelated_to_selected` is rendered with reduced opacity per the connector taxonomy section 6.4.
- Selected node and selected connector remain at full emphasis.

### 7.4 Path emphasis

Path emphasis is supported when a lens declares a multi-hop semantic flow (for example, the Authority Graph emphasising the resolved authority chain from surface to agent on selection of any chain object).

- Path emphasis layers on top of node selection. A selected node may resolve to a path; the path is rendered with `connected_to_selected` emphasis along its members.
- Path emphasis must preserve neighbouring context, per the connector taxonomy section 7.4.
- Path emphasis must not hide the connectors it is emphasising; reduced-opacity treatment applies to unrelated objects, not to the path itself.
- Lenses that do not declare path semantics fall back to the single-node connected-edge emphasis rules in section 7.3.

## 8. Inspector handoff

### 8.1 Selection drives the inspector

- Setting `selected` on a node drives an inspector update through the graph selection bridge. The inspector renders the selected node's details.
- Clearing selection drives the inspector to its empty state.
- The inspector reflects the engine's selection; the inspector does not author selection.

### 8.2 Inspector close does not clear selection

- Closing or collapsing the inspector is independent of selection state.
- A selected node remains selected when the inspector is closed.
- Re-opening the inspector while a node is selected restores the inspector to the selected node's detail view.

### 8.3 Inspector controls are separate from graph controls

- Inspector controls (close, collapse, switch tab, scroll) belong to the inspector surface.
- Graph controls (zoom, fit, centre, reframe, layout, pan) belong to the camera toolbar (section 9).
- The inspector must not render camera controls.
- The camera toolbar must not render inspector controls.

### 8.4 Handoff target for connectors

Connector selection handoff follows the controlled `handoffTarget` vocabulary in the connector taxonomy section 9.1. The platform default is `graph_inspector`. Lenses must select a target from the vocabulary and must not introduce new targets.

## 9. Graph control separation

Camera operations belong to a dedicated camera toolbar surface. The following operations are camera operations and must live on the camera toolbar, not inside the inspector, not inside the selected-object pane, not inside the node detail card:

- Zoom in and zoom out
- Fit to viewport
- Centre on selected node
- Reframe (return to default camera state)
- Reset layout
- Pan (when expressed as a button rather than a pointer gesture)

The selected-object pane and node inspector surfaces describe the selected object's state and identity. They do not control the camera. A selected node's pane may include a "centre on this node" affordance only if that affordance is rendered as a delegate that calls the camera toolbar's centre operation; the centre logic does not belong to the pane.

This separation preserves the principle in section 3: camera control, selection control, and inspector control are three independent concerns.

## 10. Visual token rules

### 10.1 Platform primary token for selected state

The selected interaction state across nodes, cards, and connectors uses the platform primary token. The token is defined in the shared platform tokens file; the implementation reference at the time of writing is `--primary` defined in `tokens.css`, with a dark-mode default value and a light-mode override.

The token reference is the standard. The specific hex value (currently `#adc6ff` in dark mode, `#004aa2` in light mode) is implementation reference. Future theme work may change the value without altering this standard.

### 10.2 Semantic overlay tokens

`warning`, `drift`, and `evidence_signal` overlays use semantic tokens from the platform's approved token set. Lenses must not introduce new colour values for these overlays. If the platform token set does not yet include a token a lens needs, the lens uses the closest existing token and the platform token set is extended in a separate tranche.

### 10.3 Non-colour cues are mandatory

Every state with a colour cue must also carry a non-colour cue:

- `selected` carries an outline ring (`box-shadow`).
- `hovered` carries a subtle elevation or border weight change.
- `focused` carries the platform `:focus-visible` outline.
- `connected_to_selected` carries default emphasis (the absence of de-emphasis is the cue).
- `unrelated_to_selected` carries reduced opacity.
- `disabled` carries reduced opacity plus a non-interactive cursor.
- `inactive` carries muted typography weight.
- `warning`, `drift`, `evidence_signal` each carry an icon or marker glyph in addition to colour.

### 10.4 No layout change on state change

State transitions must not change object size, padding, margin, or layout. Visual diff is additive (an outline ring, an opacity adjustment, an icon overlay). Size or layout shifts are forbidden because they cause neighbouring objects to reflow and break operator spatial memory.

## 11. Platform versus graph-lens responsibilities

### Platform owns

- The state vocabulary in section 4.
- The mirroring contract between the engine layer and the HTML overlay tier (section 6).
- The selection model semantics (single-select default, deselection rules, persistence rules).
- The derived-state machinery for `connected_to_selected` and `unrelated_to_selected`.
- The inspector handoff bridge (the conduit, not the inspector content).
- The graph control separation rule (section 9).
- Accessibility rules (section 12).
- The state-to-class and state-to-attribute contract (section 13).
- The approved platform token set for interaction state.
- Camera toolbar rendering and operations.

### Graph lens owns

- Which states from section 4 apply to which objects in the lens.
- The semantics behind `warning`, `drift`, and `evidence_signal` overlays in the lens context.
- Tooltip and inspector copy for hovered and selected objects.
- The lens-specific inspector content (rendered into the platform-provided inspector chrome).
- The handoff target selection per object (drawn from the connector taxonomy's `handoffTarget` vocabulary).
- Per-kind node and card visual identity (border colour, glyph) that composes with platform interaction state.
- Whether multi-select is enabled; if so, the selection model documentation.
- Path semantics, if path emphasis is declared.

No graph lens may invent a new interaction state name. No graph lens may rename or remap the state-to-class contract. No graph lens may take camera control out of the camera toolbar. No graph lens may take selection control out of the engine layer.

## 12. Accessibility

- Every interaction state must have a non-colour cue (section 10.3).
- Selected state must be conveyed to assistive technology through ARIA attributes or screen-reader text, not through colour alone.
- Keyboard focus must produce a visible focus ring via `:focus-visible`. The focus ring is distinct from the selected outline ring so a focused-but-unselected object is distinguishable from a selected-but-unfocused object.
- Hover-only information must have a keyboard-accessible equivalent. The platform may surface hover content on focus, on long-press for touch, or via an inspector summary.
- Selection must be reachable from the keyboard. The default keymap is: `Tab` to focus, `Enter` to select, `Escape` to deselect.
- Multi-select, where supported, must have keyboard equivalents documented per lens.
- Screen-reader text for a selected object must include the object kind, the object name, and the selected state. Bare visual selection without semantic announcement is insufficient.
- `prefers-reduced-motion` must suppress state-transition animations. State changes still occur; only the animated transition is suppressed.

## 13. State-to-class and state-to-attribute contract

The platform standard for representing interaction state in the DOM and engine is:

### 13.1 HTML cards

- State is represented as `[data-graph-card-state~="..."]` on the card root element.
- Multiple states are space-separated within the attribute value (for example, `[data-graph-card-state~="selected"][data-graph-card-state~="hovered"]`).
- The legacy `.selected` class on the card root is recognised by the platform during the migration period and is the form used by the Authority POC today. New lenses converge on the attribute form.
- Cards must not invent per-lens state classes (`.context-selected`, `.knowledge-active`, and similar are forbidden).

### 13.2 Engine layer (Cytoscape today)

- `node.selected()` is the source of truth for selection.
- `cy.elements(':selected')` is the canonical query for the current selection set.
- Selection events `select` and `unselect` are the canonical change signals.
- `cy.elements().unselect()` is the canonical clear-selection operation.
- Engine classes (Cytoscape's `addClass` / `hasClass`) may carry derived state such as `connected_to_selected` and `unrelated_to_selected`. Class names follow the state vocabulary in section 4.

### 13.3 Connectors

- Connector state is represented as `[data-connector-state~="..."]` on the connector overlay element when the connector has an HTML overlay tier, and as engine classes on the underlying engine edge.
- Selected and hovered states on connectors mirror the same vocabulary as nodes.
- Derived states (`connected_to_selected`, `unrelated_to_selected`) are set by the platform when a node selection changes, not by the lens.

### 13.4 Forbidden patterns

- Lens-specific state class names that bypass the vocabulary.
- Compound state names that bypass the superposition notation in section 4.
- Direct manipulation of card or connector state without going through engine-mirrored events.
- Per-lens copies of the mirroring routine that diverge from the platform contract.

## 14. Acceptance rules

Future graph interaction implementation must satisfy these rules:

- Hover treatment is visibly weaker than selection treatment at a glance.
- Selected state on cards uses the platform primary token, not a hardcoded colour.
- Selected state on cards uses a non-colour cue (outline ring).
- Selected state survives mouseout in every test case.
- Selected state survives graph re-render in every test case where the selected object remains in the projection.
- No state transition changes object size, padding, or layout.
- Clicking empty canvas clears selection.
- `Escape` from within the graph viewport clears selection.
- Closing the inspector does not clear selection.
- Camera operations do not clear selection.
- Camera controls (zoom, fit, centre, reframe, layout, pan) are not rendered inside the inspector or the selected-object pane.
- The HTML card mirrors `node.selected()` from the engine layer; the card does not author selection.
- Engine selection events drive inspector updates through the platform selection bridge.
- `connected_to_selected` and `unrelated_to_selected` are derived by the platform from selection, not authored by lenses.
- Every selectable object has a non-empty accessible name reachable through ARIA or screen-reader text.
- Keyboard focus produces a `:focus-visible` outline distinct from the selected outline.
- `prefers-reduced-motion` suppresses state-transition animations.
- No lens introduces a new interaction state name beyond the vocabulary in section 4.
- No lens introduces a new state-to-class contract beyond section 13.

## 15. Non-goals

This document does not:

- Implement graph interaction.
- Define connector semantics, families, or configuration. See the [Connector Taxonomy](strategic-graph-connector-taxonomy.md).
- Mandate a specific graph engine. Cytoscape is the current implementation; future engines must satisfy the same state-mirroring contract.
- Define node card layout, typography, or per-kind colour identity.
- Define inspector panel chrome.
- Define camera toolbar visual design.
- Define final colour token hex values.
- Alter Authority or Context graph implementations immediately.
- Define automated browser-test tooling.
- Define multi-select implementation for lenses that opt in; that is a per-lens design tranche.

## 16. Implementation roadmap

1. Adopt this standard as the platform interaction contract.
2. Assess current Authority and Context graph interaction behaviour against the standard. Identify divergences in state vocabulary, mirroring, accessibility, and control separation.
3. Define the platform mirroring routine as a shared module so future lenses do not copy Authority code.
4. Define the platform derived-state machinery for `connected_to_selected` and `unrelated_to_selected`.
5. Migrate the HTML card state representation from `.selected` class form to `[data-graph-card-state]` attribute form, preserving Authority POC behaviour during the migration.
6. Land accessibility coverage for selected, focused, and overlay states across the existing lenses.
7. Configure connector interaction behaviour per the platform mirroring routine.
8. Run source-test acceptance for the contract in section 13 (class and attribute presence, no lens-specific state names).
9. Run operator-led browser validation for rendered acceptance (visual diff at hover, select, focused; mouseout persistence; control separation).
10. Treat this standard as authoritative for the Knowledge / Semantic graph when that lens lands.

## 17. Status

Status: Proposed design standard

Owner: MIDAS graph platform

Applies to: Authority Graph, Context Graph, future graph lenses

Last updated: 2026-05-24
