# Authority Graph

The Authority Graph is the **governance view** for a business service. It
shows everything that determines whether an agent is authorised to act on a
decision surface, and what happens when authority is missing.

## Overview {#overview}

The Authority Graph is built from seven node kinds:

- [Business service](#business-service) — the root.
- [Decision surface](#decision-surface) — a named decision boundary on the
  service.
- [Authority profile](#authority-profile) — confidence / consequence
  thresholds and fail-mode for one surface.
- [Authority grant](#authority-grant) — attaches an agent to a profile.
- [Agent](#agent) — the runtime actor that executes a granted profile.
- [Fail-mode policy](#fail-mode-policy) — the rules applied when authority
  is unavailable or unclear.
- Escalation target — the recipient of escalation when a profile fails.

Edges encode *covers*, *grants*, *applies*, *escalates to* and similar
authority relationships.

The projection is deterministic and read-only. Nothing in the Explorer
mutates governance records.

## Diagnostics {#diagnostics}

The **Diagnostics** tab in the right drawer lists the projection's diagnostic
records. Each entry has:

- A severity (`info`, `warning`, `critical`).
- A diagnostic kind (e.g. `surface_missing_profile`, `grant_without_agent`,
  `dangling_escalation_target`).
- A human-readable message.
- One or more node refs (kind + id) the diagnostic applies to.

Critical and warning diagnostics typically point to gaps in authority
coverage. Info diagnostics are advisory.

## Posture {#posture}

The **Posture & Help** tab in the right drawer shows the *surface posture*
for any decision surface in the projection. Each row of the posture table
summarises one surface across six axes:

- **authority_status** — `complete`, `incomplete`, `degraded`, `uncovered`.
- **profile_status** — does the surface have an active authority profile?
- **grant_status** — is there an active grant attached to that profile?
- **agent_status** — is the grant attached to a known agent?
- **fail_mode_policy_status** — is an effective fail-mode policy in place?
- **escalation_status** — is the profile's escalation target wired up?

A surface in `complete` posture has all six axes resolved. Anything else is
worth investigating.

## Business service {#business-service}

The **business service** is the root of the Authority Graph. It is the
service-level container that owns the decision surfaces, holds an owner,
and references the default fail-mode policy via `fail_mode_policy_id`.

Inspector fields: `status`, `owner`, `service_type`, `external_ref`.
Technical fields: `fail_mode_policy_id`.

## Decision surface {#decision-surface}

A **decision surface** is a named boundary on a business service where an
authority decision must be taken (e.g. "approve transaction", "publish
content"). Each surface has at most one *effective* authority profile and
one *effective* fail-mode policy at a time.

Inspector fields: `status`, `process_id`, `effective_policy_source`,
`effective_policy_id`, `inherits_bs_policy`.
Technical fields: `version`, `business_service_id`.

`effective_policy_source` is `override` when the surface specifies its own
fail-mode policy, `inherited` when it falls back to the business service's
default, and `none` when no policy applies.

## Authority profile {#authority-profile}

An **authority profile** describes the thresholds and fail-mode that govern
a decision surface. It carries confidence / consequence thresholds and an
escalation target.

Inspector fields: `status`, `surface_id`, `escalation_target_id`,
`fail_mode`.
Technical fields: `version`, `validity_status`, `confidence_threshold`,
`consequence_threshold`, `escalation_mode`.

A surface with no active authority profile is in `incomplete` posture for
the `profile_status` axis.

## Authority grant {#authority-grant}

An **authority grant** attaches an agent to an authority profile. It can
add capability overrides or constraint overrides; it does not redefine the
profile itself.

Inspector fields: `status`, `agent_id`, `capabilities`.
Technical fields: `profile_id`, `validity_status`, `constraints`.

A grant without a known agent surfaces a `grant_without_agent` diagnostic.

## Agent {#agent}

An **agent** is the runtime actor (a model deployment, a service account, a
named system) that executes the granted profile on a decision surface.
Agents carry an operational state and an owner.

Inspector fields: `operational_state`, `type`, `owner`.
Technical fields: `model_version`.

An agent with `operational_state` other than `active` is worth checking; it
typically points at a deployment-side issue, not an authority gap.

## Fail-mode policy {#fail-mode-policy}

A **fail-mode policy** is the set of rules MIDAS applies when authority is
unavailable or unclear at evaluation time. Policies are versioned and have
an effective window. The `rule_count_by_class` field counts the rules of
each class the policy carries.

Inspector fields: `status`, `effective_date`, `business_owner`,
`technical_owner`.
Technical fields: `version`, `effective_until`, `origin`, `managed`,
`rule_count_by_class`.

A surface without an `effective_policy_id` and no inherited business-service
policy is in `incomplete` posture for the `fail_mode_policy_status` axis.
