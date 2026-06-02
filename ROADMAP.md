# ROADMAP

This document describes the direction of the Accept MIDAS project. It is intentionally high-level: items reflect current thinking, current validation evidence, and community interest. They are not binding commitments or delivery dates.

For detailed discussions, open an issue or join the conversation on GitHub Discussions.

---

## Current baseline

The following areas are now established in the repository and are no longer roadmap items in their own right.

### Runtime governance

- Deterministic `/v1/evaluate` runtime contract for governed decision evaluation.
- Runtime outcomes:
  - `accept`
  - `escalate`
  - `reject`
  - `request_clarification`
- Authority resolution across:
  - Decision Surface
  - Authority Profile
  - Grant
  - Agent
- Confidence and consequence threshold evaluation.
- Idempotency and scoped request replay protection.
- Conflict handling for reused request IDs with changed payloads.
- Evidence envelope creation for governed evaluations.
- Tamper-evident audit-chain integrity model.
- Evidence retrieval and integrity verification endpoints.

### Control plane and metamodel

- Control-plane plan/apply flow for YAML resource bundles.
- Structural metamodel support:
  - Business Service
  - Capability
  - BusinessServiceCapability
  - Process
  - Decision Surface
  - Agent
  - Authority Profile
  - Grant
- Surface and Profile lifecycle support, including review/approval paths.
- Structural read APIs for core model entities.
- Platform contract documentation covering:
  - HTTP APIs
  - runtime evaluation
  - YAML control-plane resources
  - Decision Surface contract
  - evidence envelopes
  - transactional outbox events

### Persistence and eventing

- PostgreSQL-backed runtime persistence.
- Transactional outbox pattern for downstream event emission.
- Kafka-compatible dispatcher.
- Canonical external event schema.
- Documented at-least-once delivery semantics.
- Kafka as the first supported transport.
- Azure Event Hubs validation through its Kafka-compatible endpoint.
- Batch outbox publishing and batch mark-published updates for improved dispatcher throughput.

### Observability and operations

- Health and readiness endpoints.
- Prometheus-format `/metrics`.
- Runtime, database pool, outbox backlog, and dispatcher metrics.
- Runtime readiness documentation.
- Performance documentation and benchmark interpretation.
- Backup and restore guidance.
- Observability assets:
  - Prometheus alert rules
  - Grafana dashboard
  - incident drill guidance

### Deployment

- Docker-based local development path.
- Helm chart for Kubernetes deployment.
- Kubernetes/Helm quickstart.
- kind/minikube-style local values example.
- Local Kubernetes validation using:
  - kind
  - Helm lint/template
  - temporary Postgres
  - local image load
  - health/readiness/metrics checks
  - unauthenticated `/v1/evaluate` auth check
- Helm chart security-context fix for numeric non-root execution.

### Explorer

- Graph-native Explorer for MIDAS governance visibility.
- Context Graph for operational structure.
- Authority Graph for authority relationships.
- Evidence and records views.
- Simulation support in Explorer.
- Headless runtime mode for API-only deployments.
- Optional Explorer enablement through Helm configuration.

### Release-candidate validation

- Synchronous governed evaluation path validated at 100 evaluations/sec in the Azure test environment with:
  - no HTTP failures
  - no authentication failures
  - no 5xx responses
  - no database pool saturation
- End-to-end adjudication plus event emission validated at 50 evaluations/sec.
- Higher-rate event emission remains under active dispatcher throughput remediation.

---

## Near term

### Reviewer readiness and documentation

- Finalise the consolidated platform contract guide.
- Ensure README entry points clearly explain:
  - what MIDAS is
  - how an agent uses `/v1/evaluate`
  - how MIDAS differs from authorization and policy engines
  - where to find Docker, Kubernetes, Helm, API, and control-plane guidance
- Keep Kubernetes and Helm documentation aligned with the actual chart and tested local workflow.
- Add or maintain reviewer-friendly examples for:
  - minimal control-plane YAML bundle
  - first governed evaluation
  - evidence integrity check
  - Kubernetes smoke test
- Continue improving discoverability of:
  - `docs/getting-started/kubernetes.md`
  - `charts/midas`
  - `examples/kubernetes/kind-values.yaml`
  - `docs/core/platform-contract.md`

### Dispatcher and eventing throughput

- Complete durable claim/lease support for outbox rows.
- Add claim ownership fields and retry semantics, such as:
  - `claimed_at`
  - `claimed_by`
  - `claim_expires_at`
  - `attempt_count`
- Prevent rows from being re-claimed while another dispatcher instance is publishing them.
- Add bounded dispatcher concurrency after durable leases are in place.
- Add max in-flight batch controls.
- Re-run stepped Azure throughput validation:
  - 100 eval/sec
  - 200 eval/sec
  - 300 eval/sec
- Prove event emission can remain bounded or drain reliably at higher sustained rates.
- Improve dispatcher dashboards and alerts for:
  - backlog count
  - oldest unpublished age
  - claim duration
  - publish duration
  - mark-published duration
  - publish failures
  - per-topic behaviour

### Explorer operationalisation

- Harden Explorer as a supported operational governance surface.
- Clarify Explorer versus headless mode across documentation and deployment examples.
- Promote Context Graph and Authority Graph as primary governance views.
- Improve graph interaction patterns for:
  - node actions
  - graph navigation
  - selection modes
  - evidence overlays
  - runtime records
- Add stronger support for review workflows around:
  - decision surfaces
  - authority profiles
  - grants
  - evidence envelopes
- Continue developing drift indicators and operational measures.

### Control-plane maturity

- Improve bundle dry-run output, including:
  - clearer validation messages
  - referential integrity warnings
  - active-state diffs
  - create/update/no-op distinction
- Improve lifecycle documentation and examples for:
  - Surface approval
  - Profile approval
  - deprecation
  - successor versions
- Continue hardening Profile and Grant lifecycle ergonomics.
- Add clearer examples for control-plane authoring and review.

---

## Medium term

### Tier 1 availability-capable runtime

- Establish a repeatable Tier 1-style validation programme for MIDAS.
- Validate multi-replica runtime behaviour under sustained load.
- Validate rolling revision deployment and rollback behaviour.
- Validate PostgreSQL high-availability failover behaviour.
- Validate outbox consistency during:
  - dispatcher restart
  - broker failure
  - database failover
  - runtime pod replacement
- Capture p999 latency evidence for sustained workloads.
- Define target SLO evidence for:
  - adjudication latency
  - evidence persistence
  - outbox drain time
  - recovery behaviour
- Improve replica-aware metrics collection and evidence gathering.
- Add operational drills for:
  - loss of a runtime replica
  - Postgres restart/failover
  - Kafka/Event Hubs publish failure
  - backlog recovery
  - bad revision rollback
- Clarify readiness claims by deployment profile:
  - local development
  - single-node recoverable deployment
  - multi-replica Kubernetes deployment
  - Tier 1-style availability-capable deployment

### Policy and authority model maturity

- Implement OPA/Rego as a concrete `PolicyEvaluator` implementation.
- Add policy bundle management through the control plane.
- Support policy versioning and test execution.
- Improve fail-mode policy lifecycle and inheritance.
- Strengthen policy/fail-mode evidence in envelopes.
- Expand authority analysis across:
  - surfaces
  - profiles
  - grants
  - agents
  - business services
  - capabilities
- Add cross-surface authority impact analysis.

### Decision analytics and journey context

- Add run-level linkage across related evaluations.
- Support end-to-end decision journey tracing.
- Link evaluations to escalation and review paths.
- Add analytics views for:
  - process-level decision patterns
  - capability-level activity
  - agent-level authority usage
  - outcome distribution
  - escalation drivers
- Avoid collapsing runtime evidence into isolated envelope views where a journey-level model is needed.

### Access control and platform administration

- Add finer-grained authorization for control-plane write operations.
- Improve auditability for platform-level administrative actions.
- Add clearer roles for:
  - platform operator
  - platform administrator
  - governance approver
  - governance reviewer
  - viewer
- Improve separation between runtime evaluation permissions and control-plane governance permissions.

---

## Longer term / future themes

### Multi-tenancy and workspace isolation

- Namespace or workspace isolation for multi-team deployments.
- Tenant-scoped governance resources.
- Tenant-scoped metrics, evidence, and event streams.
- Tenant-aware control-plane permissions.

### Explorer and knowledge graph

- Expand Explorer into a broader operational governance control surface.
- Add knowledge views that connect:
  - business services
  - capabilities
  - processes
  - surfaces
  - agents
  - policies
  - evidence
  - drift signals
- Add operational drift views for:
  - decision behaviour
  - evidence patterns
  - authority usage
  - profile/grant changes
  - model or agent behaviour changes
- Support graph-based impact analysis for proposed authority changes.

### Ecosystem integrations

- Integration guides for common agent and orchestration frameworks, such as:
  - LangGraph
  - Semantic Kernel
  - Temporal
  - MCP-based tool runtimes
  - OpenFGA-style authorization systems
- Additional governance-event delivery patterns.
- Webhook-style integrations where appropriate.
- Reference consumer patterns for downstream event processing.
- Managed deployment blueprints for:
  - AKS
  - EKS
  - GKE
  - OpenShift

### Policy authoring

- In-platform policy editor and test harness.
- Policy simulation against historical or synthetic evaluation inputs.
- Policy lifecycle integrated with governance review.
- Policy version comparison and promotion workflows.

### Enterprise capabilities

Some capabilities may remain outside the community repository depending on scope, maturity, or operational sensitivity:

- Emergency authority revocation with audit trail.
- Cross-surface delegated authority workflows.
- Advanced compliance reporting.
- Regulated-sector evidence packs.
- Enterprise deployment blueprints.
- Advanced multi-tenant administration.

---

## Contribution themes

Areas suitable for community contribution include:

- Kubernetes and Helm documentation improvements.
- Additional examples and tutorials.
- Policy evaluator implementations.
- Event consumer examples.
- Metrics and dashboard improvements.
- Explorer usability improvements.
- Control-plane validation improvements.
- Integration guides for agent frameworks and workflow engines.
- Local development and testing improvements.

Items marked as enterprise capabilities may remain separate from the community repository. All other items are candidates for community contribution, discussion, or issue-based refinement.