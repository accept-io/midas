# Accept MIDAS

Open-source decision governance platform for governing AI agents, automated models and human actors.

## Status

This repository is in an early scaffold phase. The initial HTTP runtime is in place, with core domain modules, persistence, and policy integration to follow.

## What is MIDAS?

MIDAS is a centralised authority orchestration platform that evaluates whether autonomous actors are authorised to execute decisions. Instead of embedding authority logic in every service, callers submit a decision request and receive an authority outcome such as:

- Execute
- Escalate
- Reject
- Request Clarification

## Quick Start

```bash
git clone https://github.com/accept-io/midas.git
cd midas
make run
```

In a second terminal:

```bash
curl http://localhost:8080/healthz
```

Example evaluation request:

```bash
curl -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "loan-approval",
    "agent_id": "lending-model-v3",
    "confidence": 0.87,
    "consequence": { "currency": "GBP", "amount": 4500 }
  }'
```

## Build

```bash
make build
make test
make docker
make dev
```

## Documentation

- [Platform Architecture](docs/architecture/platform-architecture.md)
- [Evaluation Flow](docs/architecture/evaluation-flow.md)
- [Getting Started](docs/guides/getting-started.md)
- [Decision Surfaces](docs/concepts/decision-surfaces.md)
- [Authority Model](docs/concepts/authority-model.md)
- [Operational Envelope](docs/concepts/operational-envelope.md)
- [Writing Rego Policies](docs/guides/rego-policies.md)

## Enterprise

MIDAS Enterprise adds time-bounded authority, threshold governance, escalation SLA management, RBAC, drift detection, and OpenTelemetry integration.

## Community

- Discussions: https://github.com/accept-io/community/discussions
- [Contributing](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)

## License

Apache License 2.0. See [LICENSE](LICENSE).
