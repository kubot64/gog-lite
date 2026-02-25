# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in gog-lite, please report it privately.

**Do not open a public GitHub issue for security vulnerabilities.**

### How to report

Send a report via [GitHub Security Advisories](https://github.com/kubot64/gog-lite/security/advisories/new).

Please include:
- A description of the vulnerability
- Steps to reproduce
- Potential impact

You can expect an initial response within 72 hours.

## Scope

This project handles OAuth 2.0 tokens for Google APIs. Security-relevant areas include:

- Token storage (`internal/secrets/`)
- OAuth flow (`internal/googleauth/`)
- Header injection in Gmail send (`internal/cmd/gmail.go`)
- stdin input handling (`internal/cmd/stdin.go`)
