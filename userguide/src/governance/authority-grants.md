# Authority Grants

An **authority grant** attaches a specific **agent** to an authority profile.
Without a grant, an agent has no authority on the surface even if the
profile is active.

Key fields:

- `id` — globally unique identifier.
- `profile_id` — the authority profile being granted.
- `agent_id` — the agent receiving the grant.
- `status` — `active`, `inactive`, `revoked`.
- `capabilities` — capability overrides this grant adds.
- `constraints` — constraint overrides this grant adds.
- `validity_status` — record lifecycle.

A grant whose `agent_id` points at no known agent surfaces a
`grant_without_agent` diagnostic. A grant on a retired profile surfaces a
`grant_orphaned` diagnostic.
