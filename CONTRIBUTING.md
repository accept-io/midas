# Contributing to Accept MIDAS

Thank you for your interest in contributing to Accept MIDAS — an open-source authority governance engine for autonomous decision-making systems. Contributions of all kinds are welcome: bug fixes, documentation improvements, tests, and new features.

This project follows the [CNCF Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold it.

---

## Developer Certificate of Origin (DCO)

All contributions must include a DCO sign-off. This certifies that you wrote the contribution or have the right to submit it under the project licence.

Add a sign-off to every commit:

```bash
git commit -s -m "your commit message"
```

This adds a `Signed-off-by: Your Name <your@email.com>` line to the commit. Contributions without a DCO sign-off will not be merged.

The full DCO text is available at [https://developercertificate.org](https://developercertificate.org).

---

## Prerequisites

- **Go 1.26.1+** — `go version` to check
- **Docker** — required to run the full test suite and Postgres integration tests
- **Make** — `make help` lists available targets

---

## Getting Started

```bash
git clone https://github.com/accept-io/midas.git
cd midas
./test.sh
```

`./test.sh` runs all tests inside Docker, including Postgres integration tests. It will tell you if your environment is ready.

See [TESTING.md](TESTING.md) for a full description of the test infrastructure, individual test modes, and how to run specific packages.

---

## Making a Contribution

1. Fork the repository and create a branch:

   ```bash
   git checkout -b feat/your-feature
   # or
   git checkout -b fix/your-fix
   ```

2. Make your changes following the existing code style (standard Go conventions, `go vet` clean).

3. Add or update tests. All contributions must include relevant tests. The evaluation path, reason codes, and envelope state transitions require test coverage.

4. Run the full test suite:

   ```bash
   ./test.sh
   ```

5. Open a pull request against `main`. Describe what the change does and why. Reference any related issues.

---

## Pull Request Expectations

- Describe what the change does and why in the PR description.
- Reference any related GitHub issues.
- All CI checks must pass before review.
- A maintainer will review within a reasonable timeframe — for small, well-scoped changes this is typically a few days.
- Be prepared to iterate: review feedback is normal and expected, not a rejection.

---

## Architecture Guidance

MIDAS is a modular Go monolith. Keep packages small with explicit interfaces.

A few rules that must be respected:

- **Runtime and platform layers must remain decoupled.** The runtime authority layer (`internal/decision`, `internal/envelope`, `internal/surface`) must not import or depend on the platform IAM layer (`internal/localiam`, `internal/oidc`, `internal/platformauth`). The HTTP layer (`internal/httpapi`) is the only permitted integration point.
- **New `/v1/*` API changes require prior discussion.** Any change to the public API surface — new endpoints, changed request/response fields, altered semantics — must be discussed in a GitHub issue before implementation.
- **New external dependencies require maintainer discussion.** Open an issue before introducing a new `go.mod` dependency.
- **OPA imports stay inside `internal/policy/`.** No other package may import OPA directly.
- **Reason codes are typed constants.** Do not use raw strings for reason codes — add to `internal/eval/outcome.go`.

See [docs/architecture/architecture.md](docs/architecture/architecture.md) for a full architectural overview.

---

## Reporting Issues

Use [GitHub Issues](https://github.com/accept-io/midas/issues) with the provided templates:

- **Bug report** — unexpected behaviour, crashes, incorrect outputs
- **Feature request** — new capability proposals

For security vulnerabilities, follow the process in [SECURITY.md](SECURITY.md). Do not open a public issue for security concerns.

---

## Questions

Open a [GitHub Issue](https://github.com/accept-io/midas/issues) with the `question` label, or start a [GitHub Discussion](https://github.com/accept-io/community/discussions).
