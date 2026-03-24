# Decision Surfaces

> ⚠️ **Coming Soon** — detailed documentation for this concept is planned for a future release.
> For now, see [docs/control-plane.md](../control-plane.md) for the surface lifecycle and YAML bundle format.

A decision surface is a bounded governance point at which an actor may be authorised to make or execute a consequential action. Surfaces carry metadata (domain, owner, compliance frameworks, required context keys) but do not carry authority thresholds — those live on the AuthorityProfile.
