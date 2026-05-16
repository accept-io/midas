# Business Services

A **business service** is the highest-level container in MIDAS governance. It
owns a set of decision surfaces and a default fail-mode policy.

Key fields:

- `id` — globally unique identifier.
- `status` — `active`, `inactive`, `decommissioned`.
- `owner` — business owner of the service.
- `service_type` — free-form classification.
- `external_ref` — references to external systems (catalogue ID, etc.).
- `fail_mode_policy_id` — the policy decision surfaces inherit by default.

See [Authority Graph &mdash; Business service](../graphs/authority-graph.md
#business-service) for the inspector field model.
