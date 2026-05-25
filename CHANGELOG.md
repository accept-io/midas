# Changelog

All notable changes to Accept MIDAS will be documented in this file.

This project uses [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format.
Versioned releases will follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

_No unreleased entries pending. Items below have been promoted to the v1.1.0-rc.1 release candidate._

---

## [1.1.0-rc.1] — 2026-05-25

Release candidate advancing the v1 line. Suitable for evaluation and validation while release hardening continues. Runtime governance, control-plane, evidence, and Explorer capabilities are being stabilised; graph UI surfaces continue to evolve.

### Added

- **Strategic Context graph as the default Explorer services route.** `/explorer#services` now loads the Cytoscape-backed Context graph with spatial layout by default. The legacy SVG renderer remains reachable via `?contextRenderer=legacy`; document-flow layout via `?contextLayout=flow`.
- **Cytoscape vendor asset bundled** in the repository so deployed environments load the graph renderer without external CDN dependencies.
- **In-app User Guide** at `/help/`, with a context-help button from the Explorer toolbar.
- **Runtime Evidence API** — four read-only endpoints under `/v1/evidence/*` (audit-event chain per envelope, cross-envelope search, integrity verification, composed evidence packet).
- **Governance Coverage** read model and `/v1/coverage` projection over `GOVERNANCE_CONDITION_DETECTED` and `GOVERNANCE_COVERAGE_GAP` audit events.
- **AI Systems** — first-class catalogue with versioning, bindings to capabilities, and lifecycle endpoints.
- **Escalation Targets** — governed, versioned escalation routing with the four `role` / `agent` / `surface` / `external` kinds.
- **FailModePolicy** — first-class structural entity with apply / approve / deprecate endpoints, hierarchical resolution (Surface override → BusinessService default → deployment default), three enforcement states (`evidence_only` / `dry_run` / `enforced`), and audit-chain encoding.
- **Drift Definitions** and detector lifecycle endpoints (`submit` / `approve` / `reject` / `deprecate` / `retire`); drift observations and series.
- **Authority Graph workbench** — canvas-edge tab pattern (Details / Authority / Evidence) and shared graph platform substrate (`GraphViewport`, `graph-cytoscape-engine`, `graph-camera-bus`, `graph-lens-registry`, `graph-selection-bridge`).
- **Layout-intent spacing tokens** (`--layout-page-margin`, `--layout-section-gap`, `--layout-card-padding`, `--layout-element-gap`, `--layout-inline-gap`, `--layout-tight-gap`) applied across primary Explorer views.
- **OpenAPI contract regression coverage** — bidirectional pin between Go runtime constants and the OpenAPI spec across 23 enum pairs.

### Changed

- **OpenAPI `outcome` enum corrected.** The `EvaluateResponse.outcome` enum is now `[accept, escalate, reject, request_clarification]`, matching the Go runtime and the database `outcome` CHECK constraint. Strict-schema clients should re-validate against the spec.
- **OpenAPI `Grant.status` enum corrected.** The spec-only `expired` value has been removed; the enum is now `[active, suspended, revoked]`, aligned with `authority.GrantStatus` and `GrantLifecycleResponse.status`.
- **Authority graph migrated** to the shared `GraphViewport` platform; Authority right-rail content moved to the Workbench tabs.
- **Capabilities and Drift catalogue** views gained the canonical 24px page margin (previously flush against the page edge).
- **`process_id` required** on `/v1/evaluate` in enforced structural mode.
- **Helm chart and image-tag references** aligned to the release candidate.

### Removed

- **Inference subsystem removed.** If you previously enabled `MIDAS_INFERENCE_ENABLED`, remove it. MIDAS v1 requires explicit structural configuration via control-plane apply, and `process_id` is required on evaluation requests.

### Security

- The repository uses GitHub security tooling — security policy, security advisories, private vulnerability reporting, Dependabot alerts, code scanning alerts, and secret scanning alerts — as the canonical security posture for this release candidate.
- Auth and IAM surfaces (Local IAM bootstrap, OIDC role mapping, bearer-token authenticator, scoped-permission control-plane gates) remain in place; no regressions in the auth boundary in this RC.

### Notes

- This is a release candidate, not a final stable release. Graph UI surfaces remain in active iteration; the evaluation contract (`/v1/evaluate`), envelope shape, audit-chain integrity, FailModePolicy resolution, and control-plane apply path are the stable surfaces in this RC.
- The next stable cut will retire the legacy Context SVG renderer and complete the remaining UI polish; the engineering opt-out flags (`?contextRenderer=legacy`, `?contextLayout=flow`) bridge the gap.

---

## [1.0.0] — 2026-03-28

First public release of Accept MIDAS.

### Added

- Runtime governance engine — `POST /v1/evaluate` with deterministic six-step evaluation pipeline (surface resolution, authority chain validation, context validation, threshold evaluation, policy check, outcome recording)
- Hash-chained audit trail — every evaluation produces a tamper-evident audit chain anchored in the operational envelope; integrity verifiable without application secrets
- Operational envelope — lifecycle object tracking each evaluation from receipt to closure with full evidence references; state machine: `RECEIVED → EVALUATING → OUTCOME_RECORDED → ESCALATED → CLOSED`
- Control plane — YAML bundle apply (`POST /v1/controlplane/apply`) and surface approval workflow; surfaces enter `review` on apply and become `active` only after explicit approval
- Authority model — Decision Surfaces, Authority Profiles, Authority Grants, and Agent Registry with full CRUD and versioning
- Escalation and human review — escalation queue and `POST /v1/reviews` for recording human override decisions
- Platform IAM — local username/password authentication with bootstrap admin account and forced password change on first login
- OIDC integration — provider-agnostic SSO via any OIDC-compliant identity provider; documented configuration examples for Microsoft Entra ID and Google Workspace; role mapping from external groups to internal roles
- Canonical role model — `platform.admin`, `platform.operator`, `platform.viewer`, `governance.approver`, `governance.reviewer` with separation between platform and governance responsibilities
- Explorer UI — interactive developer sandbox for evaluating demo scenarios in-browser; isolated in-memory store; not for production use
- Headless deployment mode — `server.headless: true` disables all browser-facing surfaces and platform-login routes; `/v1/*`, `/healthz`, and `/readyz` remain operational
- Config-driven deployment — `midas.yaml` canonical runtime configuration with `MIDAS_*` environment variable overrides; structural and semantic validation at startup with descriptive fatal errors
- Three documented deployment modes: headless (API-only), local platform (Explorer + local IAM), and OIDC platform (Explorer + SSO)
- OpenAPI specification — `api/openapi/v1.yaml`
- In-memory store — zero-dependency store with demo data seeding for development and testing
- PostgreSQL store — production persistence with single-file schema applied automatically at startup

### Security

- Dockerfile hardened with distroless base image (`gcr.io/distroless/base-debian12`) and nonroot runtime user
- SBOM generated in CycloneDX format (`security/sbom/`)
- Security scanning — govulncheck (clean), Trivy (0 vulnerabilities, 0 secrets), OSV scan
- `MIDAS_OIDC_CLIENT_SECRET` and bearer tokens excluded from startup log output and introspection endpoints
- Responsible disclosure process documented in `SECURITY.md`
