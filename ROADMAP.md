# ROADMAP

This document describes the direction of the Accept MIDAS project. It is intentionally high-level: items reflect current thinking and community interest, not binding commitments or delivery dates.

For detailed discussions, open an issue or join the conversation on GitHub Discussions.

---

## Current baseline

The following areas are now established in the repository and are no longer roadmap items in their own right:

- Structural layer for MIDAS V2:
  - Capability
  - Process
  - Surface → Process linkage
- Control-plane support for structural entities
- Structural read APIs
- Helm chart for Kubernetes deployment
- External eventing baseline:
  - canonical external event schema
  - documented delivery semantics
  - outbox-backed external event emission
  - Kafka as the first supported transport
  - sink configuration model

The roadmap below focuses on what remains to be matured or introduced beyond that baseline.

---

## Near term

**Observability**
- Prometheus metrics endpoint for evaluation throughput, outcome distribution, and escalation queue depth
- Structured trace context propagation (OpenTelemetry) through the evaluation pipeline

**Control plane maturity**
- Profile and grant lifecycle endpoints on par with surface lifecycle
- Bundle dry-run improvements, including referential integrity warnings and clearer diff output against active state

**Authority and runtime clarity**
- Authority Graph as a first-class runtime artefact
- Formalised simulation mode as a first-class platform capability, not only an Explorer concern

**Eventing expansion**
- Broader lifecycle coverage for external events beyond the current minimal baseline
- Additional transport support beyond Kafka
- Reference consumer patterns and integration guidance for downstream consumers

---

## Medium term

**Execution linkage and journey context**
- Run-level linkage across related decision evaluations
- End-to-end decision journey and escalation-path tracing across related evaluations and reviews

**Decision analytics**
- A first-class analytics model for process-, capability-, actor-, and journey-level analysis
- Runtime linkage needed to support journey analytics without collapsing everything into isolated envelope views

**External policy integration**
- OPA/Rego policy evaluation as the primary planned implementation of the `PolicyEvaluator` abstraction
- Policy bundle management via the control plane

**Access control**
- Finer-grained control over control-plane write operations
- Auditability for platform-level administrative actions

---

## Longer term / future themes

**Multi-tenancy**
- Namespace or workspace isolation for multi-team deployments

**Policy authoring**
- In-platform policy editor and test harness
- Policy version management integrated with governance workflows

**Ecosystem integrations**
- Additional governance-event delivery patterns, including webhook-style integrations where appropriate
- Integration guides for common AI agent frameworks and orchestration platforms

**Enterprise capabilities** *(separate repository)*
- Emergency authority revocation with audit trail
- Cross-surface authority delegation
- Advanced compliance reporting

---

Items marked as enterprise capabilities are out of scope for this community repository. All other items are candidates for community contribution.