# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Accept MIDAS, please report it responsibly by emailing **team@accept.io**.

Do not open a public GitHub issue for security vulnerabilities.

We will respond within 48 hours.

## Supported Versions

| Version       | Supported                              |
|---------------|----------------------------------------|
| 1.1.0-rc.1    | ✅ Current release candidate (evaluation) |
| 1.0.x         | Historical early releases (superseded; not actively maintained) |
| < 1.0         | ❌ Unsupported                          |

## Security Scanning

MIDAS uses GitHub security tooling as the canonical security posture for the repository:

- **Security policy** (this file) and **private vulnerability reporting** for responsible disclosure
- **Security advisories** for tracked vulnerabilities
- **Dependabot alerts** for dependency vulnerabilities
- **Code scanning alerts** for source-level findings
- **Secret scanning alerts** for credentials in commits
- **License compliance:** dependency licences are limited to BSD / MIT / Apache-2.0
- **SBOM:** CycloneDX format available in `security/sbom/`

Release validation verifies the GitHub security posture before each release candidate is cut. See the repository's GitHub Security tab for the current set of advisories, alerts, and scanning results.