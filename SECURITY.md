# Security Policy

## Supported Versions

**Maturity: alpha.** The support entries below concern security-report
handling; they do not certify production readiness or a stable API. Qualification
and release promotion follow [VERSIONING.md](VERSIONING.md). Existing tags
remain immutable. Fixes may require upgrading to a newer prerelease; no alpha
long-term-support or automatic backport commitment is made.

| Version | Supported          |
| ------- | ------------------ |
| 0.3.x   | :white_check_mark: |
| < 0.3   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately to avoid exposing it to the public.

**Do not open a public issue.**

Instead, email [sploitzberg@proton.me](mailto:sploitzberg@proton.me).

Please include:
- A description of the vulnerability
- Steps to reproduce the vulnerability
- Affected versions
- Any potential impact or exploit

We will:
- Acknowledge receipt of the report within 48 hours
- Provide a detailed response within 7 days
- Work with you to understand and resolve the issue
- Coordinate disclosure of the vulnerability

## Security Best Practices

This project follows security best practices including:
- Go vulnerability checking in CI; Dependabot security updates are controlled by GitHub repository settings
- Security scanning via gosec
- Secret scanning in CI/CD pipeline
- Dependency updates reviewed with the normal CI checks

For more information on our security practices, see the [CI guide](CONTRIBUTING.md#ci).
