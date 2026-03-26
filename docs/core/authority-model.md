# Authority Model

MIDAS evaluates whether a given actor is authorised to act on a given decision surface within a defined operational envelope. Authority flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

The surface says what is governed. The profile says under what conditions. The grant says which agent. Evaluation lookup flows the opposite direction: MIDAS receives an agent ID and surface ID, resolves the grant, and from the grant resolves the profile.

---

## Core entities

### Decision Surface

A decision surface represents a governed business decision boundary — for example, "Retail Car Loan Approval" or "Customer Refund Authority."

A surface defines **what** is governed:

- Name and business domain
- Business and technical owners
- Required context keys (what the evaluation request must provide)
- Consequence types and compliance frameworks
- Status and effective date

A surface does **not** carry authority thresholds or policy configuration. Those live on the Authority Profile.

**Status lifecycle:** `draft → review → active → deprecated → retired`

Surfaces enter `review` when applied via the control plane. They become `active` only after explicit approval. There is no `inactive` status.

Surfaces are versioned. Version resolution selects the version where `status = active` and `effective_from <= evaluation_timestamp`.

### Authority Profile

An authority profile defines **how much authority** is permitted on a surface and under what conditions. A surface can have multiple profiles — for example, "Standard Lending Authority" (£5k limit, 0.85 confidence) and "Elevated Lending Authority" (£25k limit, 0.92 confidence).

A profile carries:

| Field | Purpose |
|-------|---------|
| `confidence_threshold` | Minimum confidence score for autonomous execution (`[0.0, 1.0]`) |
| `consequence_threshold` | Maximum consequence value (monetary amount or risk rating) |
| `consequence_type` | How to interpret the consequence: `monetary` or `risk_rating` |
| `policy_reference` | Which policy bundle applies (if any) |
| `escalation_mode` | How escalations are handled: `auto` or `manual` |
| `fail_mode` | Behaviour on policy errors: `open` (continue) or `closed` (escalate) |
| `required_context_keys` | Context keys the request must supply |

Profiles are versioned with effective dates so that threshold and policy changes are traceable.

### Agent

An agent is any autonomous actor that makes decisions: an AI model, an automated service, or a human operator acting within governed limits.

An agent carries:

- Unique identifier and name
- Type: `ai`, `service`, or `operator`
- Owner (team or system)
- Model version and endpoint
- Operational state: `active`, `suspended`, or `revoked`

### Authority Grant

A grant is a **thin link** between an agent and an authority profile. It says "this agent is authorised to operate under this profile's conditions on this surface."

Grants carry **no governance semantics** — no thresholds, no policy, no escalation rules. All of that lives on the profile. A grant's only job is to connect an agent to a profile.

Grant fields: `agent_id`, `profile_id`, `granted_by`, `effective_date`, `status` (`active`, `revoked`, `expired`).

---

## The authority chain

The relationship flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

Evaluation lookup runs the opposite direction:

```
Agent → AuthorityGrant → AuthorityProfile → DecisionSurface
```

This separation keeps agent identity independent from business authority. Governance configuration is reusable across agents:

- When swapping a model version (e.g. `lending-model-v3` → `lending-model-v4`), point the new agent at the same profile. The governance policy is unchanged.
- When two agents need different authority on the same surface, create two profiles. Point each agent's grant at the appropriate profile.

---

## Authority chain validation

At evaluation time, MIDAS verifies that the resolved grant's profile belongs to the requested surface (`grant.profile.surface_id == request.surface_id`). This guards against data corruption and race conditions.

If inconsistent → `reject / GRANT_PROFILE_SURFACE_MISMATCH`

---

## Version resolution

Version resolution applies to both surfaces and profiles. The resolved version is the one where:

- `status = active`
- `effective_from <= evaluation_timestamp`

If no active version exists at evaluation time → the entity is treated as not found.

Resolved version identifiers (`surface_id + surface_version`, `profile_id + profile_version`) are persisted onto the evaluation envelope. This makes every decision fully auditable — you can always reconstruct the exact authority conditions applied.

---

## See also

- [`docs/control-plane.md`](../control-plane.md) — how to apply and approve surfaces, profiles, grants, and agents via YAML bundles
- [`docs/operations/deployment.md`](../operations/deployment.md) — full surface lifecycle walkthrough
- [`docs/core/runtime-evaluation.md`](runtime-evaluation.md) — how the authority chain is used during evaluation
- [`docs/architecture/why-the-schema-looks-like-this.md`](../architecture/why-the-schema-looks-like-this.md) — schema design rationale
