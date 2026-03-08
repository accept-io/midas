
# Why the Schema Looks Like This

Accept MIDAS stores authority configuration and runtime evaluation evidence using a deliberately structured relational schema. The schema is designed to support the core principles of the MIDAS authority model:

1. Authority is defined independently from agent identity.
2. Governance configuration evolves over time and must be versioned.
3. Runtime evaluations must produce deterministic evidence that can be audited later.
4. Operational data references configuration rather than duplicating it.

Because of these requirements, the schema may appear different from a typical CRUD application schema.

---

# The Authority Model

MIDAS governs whether an actor is permitted to execute a specific decision. Authority relationships flow in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

Each layer has a distinct responsibility.

| Layer | Purpose |
|------|--------|
| DecisionSurface | Defines **what decision is governed** |
| AuthorityProfile | Defines **how much authority is granted** |
| AuthorityGrant | Links **an agent to an authority profile** |
| Agent | Represents the **actor performing the decision** |

This separation keeps governance configuration independent from agent identity.

---

# Decision Surfaces

A **Decision Surface** represents a governed business decision, for example:

- Retail loan approval
- Customer refund authority
- Fraud transaction blocking

A surface contains metadata describing the decision:

- name
- business domain
- business owner
- lifecycle state

Decision surfaces **do not contain thresholds or policy rules**.

Those belong to authority profiles.

---

# Authority Profiles

An **Authority Profile** defines the conditions under which an agent may act on a decision surface.

Profiles contain:

- confidence threshold
- consequence threshold
- consequence type
- policy reference
- escalation mode
- fail mode
- required context fields

Multiple profiles can exist per decision surface.

Example:

| Surface | Profile | Confidence | Consequence Limit |
|-------|--------|-----------|----------------|
| Loan Approval | Standard Authority | 0.85 | £5,000 |
| Loan Approval | Elevated Authority | 0.92 | £25,000 |

This allows different agents to operate with different authority levels.

---

# Authority Grants

A **Grant** links an agent to an authority profile.

```
agent_authorizations
agent_id → profile_id
```

Grants carry **no governance rules** themselves.

All governance semantics live on the authority profile.

This design allows:

- governance rules to evolve without rewriting grants
- new agents to inherit existing authority profiles
- agents to be swapped or upgraded without redefining authority

---

# Agents

An **Agent** represents any autonomous actor:

- AI model
- automated service
- human operator

Agents contain operational metadata such as:

- owner
- model version
- endpoint
- operational state

Agents do **not contain governance logic**.

Authority is always defined externally via profiles.

---

# Versioning

Decision surfaces and authority profiles are versioned.

Example:

```
decision_surfaces
id: loan_auto_approval
version: 1
```

```
authority_profiles
id: profile-loan-low
version: 1
```

New versions are created when governance configuration changes.

Version resolution follows a deterministic rule:

> resolve the latest version where `effective_date <= evaluation time`

This guarantees that the exact configuration used during an evaluation can always be reconstructed.

---

# Why Grants Do Not Reference Profile Versions

Authority grants link to the **logical profile ID**, not a specific version.

```
agent_authorizations
agent_id → profile_id
```

During evaluation, the orchestrator resolves the active profile version.

Advantages:

- profile thresholds can evolve without rewriting grants
- model upgrades do not require governance changes
- governance configuration remains stable while implementation evolves

---

# Operational Envelopes

Every evaluation creates an **Operational Envelope**.

The envelope records:

- request ID
- resolved surface and profile versions
- agent ID
- authority outcome
- reason code

Instead of copying configuration values, envelopes store **references** to resolved versions:

```
surface_id + surface_version
profile_id + profile_version
```

This ensures that the exact thresholds and policies applied during evaluation can always be reconstructed.

---

# Why Consequence Is Stored on Profiles

Authority thresholds belong on **Authority Profiles**, not on surfaces or grants.

| Entity | Responsibility |
|------|----------------|
| Surface | Defines the decision |
| Profile | Defines authority thresholds |
| Grant | Assigns a profile to an agent |

This separation enables:

- multiple authority levels per surface
- reusable governance templates
- consistent authority delegation across agents

---

# Why Evidence Is Stored by Reference

Operational envelopes store references instead of duplicating configuration data.

Example:

```
surface_id
surface_version
profile_id
profile_version
```

This avoids data duplication while ensuring that audit systems can reconstruct the exact configuration used during evaluation.

---

# Why the Schema Is Relational

MIDAS uses PostgreSQL because the authority model is naturally relational.

Relationships include:

- surfaces have many profiles
- profiles can be granted to many agents
- agents can hold multiple grants
- evaluations reference multiple entities

Relational constraints ensure governance integrity.

---

# Summary

The MIDAS schema is designed to support:

- clear separation of governance and identity
- deterministic evaluation and auditability
- safe evolution of authority rules over time
- operational traceability of every decision

The schema prioritizes **governance integrity and explainability** over convenience.
