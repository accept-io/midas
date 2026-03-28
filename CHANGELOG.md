# Changelog

All notable changes to Accept MIDAS will be documented in this file.

This project uses [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format.
Versioned releases will follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

### Added

- Runtime governance engine — `POST /v1/evaluate` with deterministic six-step evaluation pipeline (surface resolution, authority chain validation, context validation, threshold evaluation, policy check, outcome recording)
- Hash-chained audit trail — every evaluation produces a tamper-evident audit chain anchored in the operational envelope; integrity verifiable without application secrets
- Operational envelope — lifecycle object tracking each evaluation from receipt to closure with full evidence references; state machine: `RECEIVED → EVALUATING → OUTCOME_RECORDED → ESCALATED → CLOSED`
- Control plane — YAML bundle apply (`POST /v1/controlplane/apply`) and surface approval workflow; surfaces enter `review` on apply and become `active` only after explicit approval
- Authority model — Decision Surfaces, Authority Profiles, Authority Grants, and Agent Registry with full CRUD and versioning
- Escalation and human review — escalation queue and `POST /v1/reviews` for recording human override decisions
- Platform IAM — local username/password authentication with bootstrap admin account and forced password change on first login
- OIDC integration — Entra ID and Google Workspace SSO via platform OIDC; role mapping from external groups to internal roles
- Canonical role model — `platform_admin`, `platform_operator`, `platform_viewer`, `governance_approver`, `governance_reviewer` with separation between platform and governance responsibilities
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
- Continuous security scanning — govulncheck (clean), Trivy (0 vulnerabilities, 0 secrets), OSV scan
- `MIDAS_OIDC_CLIENT_SECRET` and bearer tokens excluded from startup log output and introspection endpoints
- Responsible disclosure process documented in `SECURITY.md`
