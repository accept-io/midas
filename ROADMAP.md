# ROADMAP

This document describes the direction of the Accept MIDAS project. It is intentionally high-level: items reflect current thinking and community interest, not binding commitments or delivery dates.

For detailed discussions, open an issue or join the conversation on GitHub Discussions.

---

## Near term (post-1.0)

**Observability**
- Prometheus metrics endpoint for evaluation throughput, outcome distribution, and escalation queue depth
- Structured trace context propagation (OpenTelemetry) through the evaluation pipeline

**Control plane maturity**
- Profile and grant lifecycle endpoints (approve, deprecate) on par with surface lifecycle
- Bundle dry-run improvements: referential integrity warnings, diff view against current active state

**Operational hardening**
- Kubernetes deployment reference manifests (Deployment, Service, ConfigMap, readiness/liveness probes)
- Helm chart (community contribution welcome)
- Graceful schema migration support for minor version upgrades

---

## Medium term

**External policy integration**
- OPA/Rego policy evaluation — the `PolicyEvaluator` interface is already defined; OPA integration is the primary planned implementation
- Policy bundle management via the control plane

**Eventing improvements**
- At-least-once delivery guarantees documented and tested under failure scenarios
- Additional publisher backends beyond Kafka (e.g. NATS, AWS SNS/SQS) via the `Publisher` interface
- Consumer SDK or reference consumer for common event processing patterns

**Access control**
- Role-based access control on control-plane write operations (currently authenticated users with correct role can apply any bundle)
- Audit log for platform-level actions (user management, OIDC config changes)

---

## Longer term / future themes

**Multi-tenancy**
- Namespace or workspace isolation for multi-team deployments

**Policy authoring**
- In-platform policy editor and test harness
- Policy version management integrated with the governance workflow

**Ecosystem integrations**
- Webhook delivery for governance events (complement to Kafka)
- Integration guides for common AI agent frameworks and orchestration platforms

**Enterprise capabilities** *(separate repository)*
- Emergency authority revocation with audit trail
- Cross-surface authority delegation
- Advanced compliance reporting

---

Items marked as enterprise capabilities are out of scope for this community repository. All other items are candidates for community contribution.
