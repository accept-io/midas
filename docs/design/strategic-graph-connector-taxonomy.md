# Strategic Graph Connector Taxonomy

Status: Proposed design standard (hardened)

Owner: MIDAS graph platform

Applies to: Authority Graph, Context Graph, future graph lenses

Last updated: 2026-05-24

## 1. Purpose

This document defines the reusable connector taxonomy, visual grammar, and interaction model for MIDAS strategic graphs.

Connectors are first-class semantic graph objects. A connector is not merely a visual line between two nodes. It explains why two graph objects are related, what the relationship means, and how the operator should read that relationship in the current graph lens.

This standard applies across the Authority Graph, Context Graph, and future graph lenses. Implementations must not invent unrelated connector languages per graph. A graph lens may configure relationship semantics, but the shared graph platform owns the common connector grammar and interaction rules.

## 2. Scope

This standard covers:

- Authority graph connectors.
- Context graph connectors.
- Future Knowledge and Semantic graph connectors.
- Connector visual grammar.
- Connector interaction model.
- Connector platform responsibilities.
- Graph-lens configuration responsibilities.

This standard does not cover:

- Node card design.
- Inspector panel implementation.
- Graph layout algorithms.
- Backend schema changes.
- Cytoscape-specific implementation details.
- Final colour token hex values.

## 3. Design Principles

- Connectors are semantic, not decorative.
- Direction must mean something. A directed relationship must read naturally from source to target.
- Thickness must mean something. It must not be used as decoration.
- Labels must be short and meaningful.
- Hover should reveal detail without cluttering the default graph.
- Selected nodes should emphasise directly connected relationships.
- Dense relationships should be summarised or bundled before they overwhelm the graph.
- Graph lenses configure semantics, but the platform owns common rendering and interaction rules.
- Accessibility must not depend on colour alone.
- Connector state must be predictable across graph lenses.
- The same semantic family should feel visually related across Authority, Context, and future graph lenses.
- The same semantic variable must not be encoded in more than one visual channel without explicit justification.
- Every connector has one canonical direction. Inverse forms exist only in display copy, never as separate connector types.

## 4. Connector Families

### 4.1 Structural Connectors

Structural connectors define composition, containment, hierarchy, membership, or graph structure.

Examples:

- `contains`
- `part_of`
- `has_surface`
- `has_capability`
- `has_process`

Use structural connectors when the relationship answers: "What is this made of?" or "Where does this belong?"

### 4.2 Dependency Connectors

Dependency connectors express dependency, usage, support, or operational reliance.

Examples:

- `depends_on`
- `uses`
- `supports`
- `provides_input_to`
- `consumes_from`

Use dependency connectors when the relationship answers: "What does this need?" or "What does this support?"

### 4.3 Authority and Governance Connectors

Authority and governance connectors express governance, authority, policy, grant, or control.

Examples:

- `authorises`
- `governs`
- `uses_profile`
- `has_grant`
- `constrains`
- `has_fail_mode_policy`
- `escalates_to`

Use authority and governance connectors when the relationship answers: "What control applies?" or "Who or what grants authority?"

Note: active-voice forms (`authorises`, `governs`, `constrains`) are canonical. Passive-voice forms (`authorised_by`, `governed_by`, `constrained_by`) are display copy only and must not appear as connector type ids. See section 6.1 and section 15.

### 4.4 Evidence Connectors

Evidence connectors express evidence, proof, observation, source, or audit linkage.

Distinguishing test: an evidence relationship asserts that one object provides epistemic support for another (proof, observation, attestation, audit trail). If the relationship is interpretive ("X explains Y") rather than evidential ("X proves Y"), classify it under semantic and contextual connectors (4.7).

Examples:

- `evidences`
- `observes`
- `supports_claim`
- `derives_from`
- `audits`

Use evidence connectors when the relationship answers: "What proves this?" or "What observation supports this?"

Canonical direction is from the evidence source to the supported claim or object. The phrase "evidenced by" exists in tooltips and inspector copy, but the connector type is the active-voice form (e.g. `signal_evidences_outcome`).

### 4.5 Runtime and Operational Connectors

Runtime and operational connectors express runtime activity, execution, telemetry, or operational event linkage.

Examples:

- `emits`
- `triggers`
- `invokes`
- `produces`
- `consumes`
- `reports_to`

Use runtime and operational connectors when the relationship answers: "What happens at runtime?" or "What operational flow links these objects?"

### 4.6 Drift, Risk, and Exception Connectors

Drift, risk, and exception connectors express deviation, risk, anomaly, exception, or control failure.

Examples:

- `drifts_from`
- `violates`
- `conflicts_with`
- `missing_evidence_for`
- `exception_to`
- `at_risk_due_to`

Use drift, risk, and exception connectors when the relationship answers: "What is wrong, missing, drifting, or unsafe?"

### 4.7 Semantic and Contextual Connectors

Semantic and contextual connectors express meaning, similarity, explanation, or contextual association.

Distinguishing test: a semantic relationship asserts interpretive or conceptual association (meaning, similarity, explanation). It does not assert epistemic proof. If the relationship offers proof or attestation, classify it under evidence (4.4).

Examples:

- `relates_to`
- `explains`
- `similar_to`
- `references`
- `contextualises`
- `informs`

Use semantic and contextual connectors when the relationship answers: "Why are these concepts associated?" or "What meaning links them?"

## 5. Lens-Specific Connector Taxonomy

### 5.1 Authority Graph

| Connector type | Source kind | Target kind | Label | Direction | Family | Intended meaning |
|---|---|---|---|---|---|---|
| `business_service_has_surface` | Business service | Decision surface | has surface | directed | Structural | The business service includes or owns a decision surface. |
| `surface_uses_profile` | Decision surface | Authority profile | uses profile | directed | Authority and governance | The decision surface evaluates requests under an authority profile. |
| `profile_has_grant` | Authority profile | Grant | has grant | directed | Authority and governance | The profile contains a grant that contributes allowed authority. |
| `grant_authorises_agent` | Grant | Agent | authorises | directed | Authority and governance | The grant authorises an agent or actor under defined conditions. |
| `surface_has_fail_mode_policy` | Decision surface | Fail mode policy | fail mode policy | directed | Authority and governance | The surface has a directly assigned fail-mode policy. |
| `business_service_has_fail_mode_policy` | Business service | Fail mode policy | fail mode policy | directed | Authority and governance | The service provides a default or inherited fail-mode policy. |
| `profile_escalates_to` | Authority profile | Escalation target | escalates to | directed | Authority and governance | Decisions outside the profile route to an escalation target. |

### 5.2 Context Graph

| Connector type | Source kind | Target kind | Label | Direction | Family | Intended meaning |
|---|---|---|---|---|---|---|
| `service_contains_capability` | Business service | Capability | contains | directed | Structural | The service contains or exposes a capability. |
| `service_depends_on_capability` | Business service | Capability | depends on | directed | Dependency | The service depends on a capability provided elsewhere. |
| `capability_supports_process` | Capability | Process | supports | directed | Dependency | The capability supports a business or operational process. |
| `process_uses_surface` | Process | Decision surface | uses surface | directed | Dependency | The process uses a decision surface. |
| `decision_surface_observes_signal` | Decision surface | Runtime signal | observes | directed | Runtime and operational | The surface is associated with a runtime or telemetry signal. |
| `signal_evidences_outcome` | Runtime signal | Outcome | evidences | directed | Evidence | The signal provides evidence about an outcome. |
| `decision_surface_has_evidence_signal` | Decision surface | Evidence signal | has evidence | directed | Evidence | The decision surface has an associated evidence signal. |
| `capability_has_drift_signal` | Capability | Drift signal | drift signal | directed | Drift, risk, and exception | The capability has an associated drift signal. |
| `process_emits_activity` | Process | Activity event | emits | directed | Runtime and operational | The process emits a runtime activity event. |

Generic `node_*` and `context_*` connector types are not permitted. If a relationship cannot be expressed using a specific domain kind (Business service, Capability, Process, Decision surface, Runtime signal, etc.), the relationship must be classified as a fallback connector and explicitly marked `fallback: true` in its configuration. Fallback connectors require justification and a retirement plan.

### 5.3 Future Knowledge / Semantic Graph

This section is provisional until the Knowledge graph lens is implemented.

| Connector type | Source kind | Target kind | Label | Direction | Family | Intended meaning |
|---|---|---|---|---|---|---|
| `concept_relates_to_concept` | Concept | Concept | relates to | undirected_associative | Semantic and contextual | Two concepts are meaningfully associated. |
| `concept_explains_policy` | Concept | Policy | explains | directed | Semantic and contextual | A concept explains the purpose or interpretation of a policy. |
| `evidence_supports_claim` | Evidence | Claim | supports | directed | Evidence | Evidence supports a claim. |
| `claim_conflicts_with_claim` | Claim | Claim | conflicts with | bidirectional_semantic | Drift, risk, and exception | Two claims are in mutual tension or conflict. |
| `source_references_source` | Source | Source | references | directed | Semantic and contextual | One source cites or references another. (Reclassified from evidence: a citation asserts reference, not proof.) |
| `pattern_similar_to_pattern` | Pattern | Pattern | similar to | undirected_associative | Semantic and contextual | Two patterns share meaningful structure or behaviour. |

## 6. Visual Grammar

### 6.1 Direction and Arrowheads

- Use arrowheads only when relationship direction has semantic meaning.
- Avoid arrows for purely associative relationships.
- Arrow direction must match the relationship phrase.
- Connector type ids must use active voice (`X verbs Y`), not passive voice (`Y is verbed_by X`).
- Each semantic relationship has exactly one canonical directed connector type.
- Inverse connector types are forbidden. Do not create `profile_used_by_surface` as a sibling of `surface_uses_profile`.
- Inverse readings, when needed, render as display copy in tooltips and inspector summaries. The underlying connector type id remains canonical.

### 6.2 Labels

- Labels must be short.
- Labels should use human-readable verbs or compact relationship phrases.
- Persistent labels should be used sparingly.
- Hover labels are preferred for dense graphs.
- Never expose raw internal edge ids as user-facing labels.
- Connector labels should fit the sentence: "source [label] target."

#### Controlled vocabulary — `labelPolicy`

A connector's label display behaviour is one of:

- `hidden` — never display a label.
- `hover` — show label only on hover. **Default.**
- `selected` — show label only when the connector or one of its endpoints is selected.
- `persistent` — always show the label.
- `bundled_count` — when bundled, show an aggregate count instead of individual labels.

`persistent` requires explicit justification in the connector configuration.

### 6.3 Thickness and Weight

- Thickness must not be decorative.
- Thickness may represent magnitude, count, confidence, impact, or event volume only when explicitly defined.
- Weighted relationships must expose the meaning in a tooltip, legend, or inspector.
- If a graph lens cannot explain the weight, it must use the default connector thickness.

#### Controlled vocabulary — `weightPolicy`

A connector's weight encoding is one of:

- `none` — uniform default thickness. **Default.**
- `magnitude` — thickness encodes a magnitude value.
- `count` — thickness encodes an event or relationship count.
- `confidence` — thickness encodes a confidence score.
- `impact` — thickness encodes operational or business impact.
- `event_volume` — thickness encodes runtime event frequency.

A graph view may use thickness for at most one of these meanings at a time. If multiple quantitative meanings exist, only one drives thickness; the others must appear in tooltip or inspector content. The weight policy must be exposed through the connector's tooltip and the graph legend.

### 6.4 Opacity and De-Emphasis

- Dim unrelated connectors when a node or path is selected.
- Inactive or deprecated relationships may use reduced opacity.
- Low opacity must not be the only accessibility signal.
- De-emphasis must not hide critical relationship context unexpectedly.

### 6.5 Line Style

- Solid lines represent normal active relationships.
- Dashed lines may represent inferred, provisional, missing, or indirect relationships only if the meaning is documented.
- Dotted or patterned lines must have a legend or tooltip explanation.
- A line pattern must not change meaning between graph lenses.

### 6.6 Colour

- Colour should be semantic and token-based.
- Colour must not be the only indicator.
- Avoid per-graph arbitrary colour schemes.
- Graph lenses may map connector families to approved platform tokens only.
- Graph lenses must not introduce arbitrary colours.
- If a connector family has no approved platform token, it must use the default connector token until the platform token set is extended.
- Colour families should remain stable for connector families across strategic graphs.

### 6.7 Bundling and Summarisation

- Dense connectors should be bundled or summarised when visual density becomes high.
- Bundled connectors should show a count or short label.
- Expansion should be available through hover, selection, or graph interaction where appropriate.
- Bundling must preserve semantic meaning. It must not merge unrelated relationship families into an ambiguous line.

#### Controlled vocabulary — `bundlePolicy`

A connector's bundling behaviour is one of:

- `none` — never bundle.
- `by_family` — bundle connectors that share a connector family between the same endpoints.
- `by_source_target_kind` — bundle connectors that share source and target kinds between the same endpoints.
- `by_target_cluster` — bundle when the target endpoint is part of a visual cluster.

`bundleThreshold` is the minimum edge count at which bundling activates for a given policy. The platform provides defaults; graph lenses may override per connector type.

### 6.8 Do Not Encode

The same semantic variable must not be encoded simultaneously in colour, thickness, and line style without explicit justification recorded in the connector configuration.

Acceptable encoding combinations:

- One variable per channel (colour, thickness, line style, opacity).
- Redundant encoding (colour plus line style) only when justified by accessibility (e.g. colour-blind support).

Forbidden by default:

- Same variable on colour and thickness.
- Same variable on thickness and line style.
- Same variable on colour and line style without accessibility justification.

When in doubt: pick one channel and use other channels for orthogonal information (state, family, direction).

## 7. Connector Interaction Model

### 7.1 Hover

Hover should show:

- Relationship label.
- Source and target.
- Short explanation.
- Weight, count, or state where applicable.

Hover content should be compact and should not require the operator to leave the graph canvas.

### 7.2 Selection

Selecting a connector should show a compact relationship summary.

It must not open a large unrelated workbench by default. If deeper inspection is available, selection may hand off to the graph inspector platform or a bottom tray.

### 7.3 Node Selection

Selecting a node should:

- Emphasise directly connected connectors.
- Dim unrelated connectors.
- Optionally group connectors by type.
- Preserve important relationship context.

Node selection must not unexpectedly hide connectors that explain the selected node.

### 7.4 Path Emphasis

The platform may support selected path emphasis for multi-hop flows.

Path emphasis must preserve readability. A path should highlight the selected flow without making neighbouring context impossible to understand.

### 7.5 Inspector Handoff

Connector detail should integrate with the graph inspector platform or bottom tray where appropriate.

Do not create separate connector-specific inspector chrome unless a future design explicitly requires it.

#### Controlled vocabulary — `handoffTarget`

A connector's handoff target on selection is one of:

- `none` — no handoff; selection shows compact summary only.
- `graph_inspector` — hands off to the shared graph inspector platform. **Default.**
- `bottom_tray` — hands off to the bottom tray.
- `lens_specific_panel` — hands off to a lens-owned panel; requires explicit justification.

## 8. Platform Versus Graph Responsibilities

### Platform Owns

- Common connector rendering mechanics.
- Arrowhead conventions.
- Hover behaviour.
- Selection behaviour.
- Active and dimmed states.
- Label placement rules.
- Bundling and summarisation mechanics.
- Accessibility conventions.
- Common legend model.
- Connector interaction lifecycle.
- The controlled vocabularies for direction, labelPolicy, weightPolicy, statePolicy, bundlePolicy, and handoffTarget.
- The approved platform colour token set for connector families.

### Graph Lens Config Owns

- Relationship types (connector ids).
- Source and target kind rules.
- Connector labels.
- Connector family assignment.
- Whether direction matters (selecting from the platform's direction vocabulary).
- Weight meaning (selecting from the platform's weightPolicy vocabulary).
- State meaning (selecting from the platform's statePolicy vocabulary).
- Tooltip text.
- Compact inspector text.
- Handoff target (selecting from the platform's handoffTarget vocabulary).
- Bundle policy and threshold (selecting from the platform's bundlePolicy vocabulary).

No graph lens may create a fully separate connector visual language. No graph lens may introduce colour tokens outside the platform's approved set.

## 9. Connector Configuration Contract

The connector configuration contract is conceptual. It describes the shape that graph lenses must eventually provide to the reusable graph connector platform.

### 9.1 Controlled vocabularies

| Field | Allowed values |
|---|---|
| `direction` | `directed` \| `undirected_associative` \| `bidirectional_semantic` |
| `arrowPolicy` | `directed` \| `none` \| `bidirectional` |
| `labelPolicy` | `hidden` \| `hover` \| `selected` \| `persistent` \| `bundled_count` |
| `weightPolicy` | `none` \| `magnitude` \| `count` \| `confidence` \| `impact` \| `event_volume` |
| `statePolicy` | one or more of the state vocabulary in section 9.3 |
| `bundlePolicy` | `none` \| `by_family` \| `by_source_target_kind` \| `by_target_cluster` |
| `handoffTarget` | `none` \| `graph_inspector` \| `bottom_tray` \| `lens_specific_panel` |
| `family` | `structural` \| `dependency` \| `authority_governance` \| `evidence` \| `runtime_operational` \| `drift_risk_exception` \| `semantic_contextual` |

### 9.2 Minimum connector definition

Every connector configuration must define, at minimum:

- `id`
- `family`
- `sourceKind`
- `targetKind`
- `label`
- `direction`
- `arrowPolicy`
- `labelPolicy`
- `accessibilityLabel`

A configuration that omits any of these nine fields is invalid and must not be accepted by the platform.

### 9.3 Controlled state vocabulary — `statePolicy`

Connector states are drawn from this closed set:

- `active`
- `inactive`
- `deprecated`
- `inferred`
- `provisional`
- `missing`
- `violated`
- `drifted`
- `at_risk`
- `selected`
- `dimmed`

A connector's `statePolicy` is one or more values from this list, representing the states the connector type can carry. Lens-specific state names that do not map to this vocabulary are forbidden.

### 9.4 Configuration shape

```text
StrategicConnectorType {
  id
  family
  sourceKind
  targetKind
  label
  direction
  arrowPolicy
  weightPolicy
  statePolicy
  labelPolicy
  bundlePolicy
  bundleThreshold
  hoverSummary
  selectionSummary
  handoffTarget
  accessibilityLabel
  fallback
}
```

### 9.5 Example — Authority connector

```text
{
  id: "surface_uses_profile",
  family: "authority_governance",
  sourceKind: "decision_surface",
  targetKind: "authority_profile",
  label: "uses profile",
  direction: "directed",
  arrowPolicy: "directed",
  weightPolicy: "none",
  statePolicy: ["active", "deprecated", "dimmed", "selected"],
  labelPolicy: "hover",
  bundlePolicy: "none",
  bundleThreshold: null,
  hoverSummary: "Decision surface uses this authority profile for evaluation.",
  selectionSummary: "This surface evaluates requests under the selected authority profile.",
  handoffTarget: "graph_inspector",
  accessibilityLabel: "Decision surface uses authority profile",
  fallback: false
}
```

### 9.6 Example — Context connector

```text
{
  id: "process_uses_surface",
  family: "dependency",
  sourceKind: "process",
  targetKind: "decision_surface",
  label: "uses surface",
  direction: "directed",
  arrowPolicy: "directed",
  weightPolicy: "none",
  statePolicy: ["active", "dimmed", "selected"],
  labelPolicy: "hover",
  bundlePolicy: "by_source_target_kind",
  bundleThreshold: 8,
  hoverSummary: "Process uses this decision surface in the current context.",
  selectionSummary: "The selected process depends on this decision surface.",
  handoffTarget: "graph_inspector",
  accessibilityLabel: "Process uses decision surface",
  fallback: false
}
```

## 10. Accessibility and Readability

- Selectable connectors must have accessible names.
- Visual state must not depend on colour alone.
- Hover-only information must have an accessible equivalent.
- Labels must remain readable at normal zoom.
- Dense graphs should reduce clutter rather than overwhelm the user.
- Keyboard interaction should be considered for selectable connectors.
- Screen-reader text should describe the relationship, not only the visual endpoints.
- Legends must explain any non-obvious line style, colour, weight, or state.
- Every connector definition must include a non-empty `accessibilityLabel`.

## 11. Legend and Explainability

Every non-default connector visual rule must be explainable through one or more of:

- legend
- hover tooltip
- inspector summary
- accessible label

The platform-supplied legend exposes:

- connector family (with colour token mapping)
- line style meaning (solid, dashed, dotted, patterned)
- colour meaning (per family or per state)
- thickness meaning (only if any active connector type declares a non-`none` `weightPolicy`)
- state meaning (per state vocabulary entry that appears in the active graph)
- bundling indicator and count semantics

The legend is rendered by the platform from the active graph lens's connector configuration. Graph lenses must not render bespoke legends. If the legend needs to show graph-specific copy, the lens supplies the text through configuration, not through a separate UI surface.

## 12. Relationship Type Naming Rules

Connector type ids follow these rules:

- snake_case.
- Active voice: `source_verbs_target`. Example: `surface_uses_profile`.
- Source kind appears first, then the verb, then the target kind. Example: `business_service_has_surface`.
- Avoid passive voice: `surface_used_by_profile` is forbidden as a connector id.
- Avoid generic source or target nouns. `node_relates_to_thing` is forbidden; use a specific domain kind.
- Avoid acronyms unless the acronym is already in the platform's domain vocabulary (e.g. `aisystem`).
- One canonical id per semantic relationship. Do not create paired inverse ids.
- Connector ids must be unique across all graph lenses. If two lenses would use the same relationship, the connector type is defined once in shared configuration.

## 13. Acceptance Rules

Future connector implementation must satisfy these rules:

- Every connector has a semantic type.
- Every visible connector type has a non-empty `label`.
- Every connector definition has every field listed in section 9.2.
- Every connector definition's `accessibilityLabel` is non-empty.
- Directionality is one of the values in the controlled vocabulary (section 9.1).
- Thickness, colour, opacity, or line pattern encode meaning drawn from the controlled vocabularies; decorative encoding is forbidden.
- Selected node emphasis follows the platform-defined behaviour for active and dimmed states.
- Hover summary is non-empty for connector types with `labelPolicy: hover`.
- Bundling is configured for connector types likely to exceed `bundleThreshold` at production data densities.
- Graph-specific connector semantics are provided through configuration, not through bespoke renderer code.
- Platform rendering mechanics are not duplicated per graph lens.
- Accessibility is available for every connector state in `statePolicy`.
- Connector family assignment supports the platform-rendered legend without lens-specific overrides.
- No connector type encodes the same semantic variable in two visual channels without justification in its configuration (see section 6.8).
- No graph lens introduces colour outside the platform's approved token set.

## 14. Validation Approach

This section anticipates the constraint that MIDAS does not run browser-automation tooling in agent-driven verification loops.

Configuration validation is source-testable:

- Every connector definition has all fields in section 9.2.
- Every connector field uses a value in the controlled vocabulary for that field.
- No connector id appears in passive-voice form.
- No connector id uses generic `node_` or `context_` source/target kinds (unless `fallback: true`).
- No two connector types pair as inverses.
- Every `accessibilityLabel` is non-empty.

Rendered validation requires browser inspection:

- Arrowheads render in the direction declared.
- Labels appear under the declared `labelPolicy`.
- Bundles activate at the declared `bundleThreshold`.
- State transitions render through the declared visual channels.
- Legend output matches the active connector configuration.
- Accessibility tools surface the `accessibilityLabel` for selected connectors.

The agent verification loop covers source-testable validation. Rendered validation is the operator's manual loop and is performed in the browser. Acceptance for any connector implementation tranche requires both.

## 15. Non-Goals

This document does not:

- Implement connectors.
- Mandate a specific graph library.
- Define final CSS token hex values.
- Replace graph data contracts.
- Alter Authority or Context graph implementations immediately.
- Define graph layout algorithms.
- Define node card appearance.
- Define inspector panel chrome.
- Define automated browser-test tooling.

## 16. Implementation Roadmap

1. Adopt this hardened taxonomy as the design standard.
2. Assess current Authority and Context connector rendering against the standard.
3. Design reusable connector platform mechanics, including the platform-supplied legend.
4. Implement hover, label, state, and bundling behaviour in the shared graph platform.
5. Configure Authority connectors against the controlled vocabularies.
6. Configure Context connectors against the controlled vocabularies, retiring any generic `node_*` or `context_*` ids.
7. Add source-test acceptance for configuration validation.
8. Run operator-led browser validation for rendered acceptance.
9. Retire inconsistent connector styling across graph lenses.
10. Treat the standard as authoritative for the Knowledge / Semantic graph when that lens lands.

## 17. Status

Status: Proposed design standard (hardened)

Owner: MIDAS graph platform

Applies to: Authority Graph, Context Graph, future graph lenses

Last updated: 2026-05-24