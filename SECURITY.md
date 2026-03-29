# Security Policy

## Security bulletins

Security-related announcements for MIDAS will be communicated through the project repository and direct maintainer response where appropriate.

For urgent security matters, please contact:

- Email: team@accept.io

## Reporting a vulnerability

If you discover a security vulnerability in MIDAS, please report it responsibly by emailing **team@accept.io**.

Please do **not** open a public GitHub issue for security vulnerabilities.

Please include:
- a description of the issue
- affected version(s)
- steps to reproduce, if available
- any known mitigations or workarounds

You will receive an acknowledgement within **48 hours**.

We may follow up for additional detail as we investigate the report, reproduce the issue, and determine scope and impact.

## Disclosure process

MIDAS follows a coordinated disclosure approach. Reported vulnerabilities will be reviewed privately and remediated before public disclosure where appropriate.

## Supported versions

| Version | Supported |
|---------|-----------|
| 1.x     | ✅        |
| < 1.0   | ❌        |

## Security scanning

MIDAS undergoes regular security scanning, including:

- Go vulnerability scanning with `govulncheck`
- Dependency and container scanning with `Trivy`
- License compliance review
- SBOM generation in CycloneDX format

Security artifacts are maintained in the repository under:
- `security/sbom/`
- `security/scans/`
