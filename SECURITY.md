# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Accept MIDAS, please report it responsibly by emailing **security@accept.io**.

Do not open a public GitHub issue for security vulnerabilities.

We will respond within 48 hours.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x     | ✅        |
| < 1.0   | ❌        |

## Security Scanning

MIDAS undergoes continuous security scanning:

- **Go vulnerabilities:** govulncheck (clean - 2026-03-27)
- **Dependencies:** Trivy (0 vulnerabilities, 0 secrets)
- **License compliance:** All dependencies BSD/MIT/Apache-2.0
- **SBOM:** CycloneDX format available in `security/sbom/`

Scan results: `security/scans/`