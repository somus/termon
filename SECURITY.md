# Security policy

Use this policy to report vulnerabilities in Termon. Do not open a public issue
for security reports.

## Supported versions

Termon is early-stage. Security fixes target the default branch and the current
release line until the project starts maintaining separate release branches.

Termon ships as a single binary and a container image on GHCR. Production
credentials, including the PostHog project token and backup credentials, are
managed by Dokploy and must not be committed to the repository.

## Reporting a vulnerability

Report security issues through
[GitHub Security Advisories](https://github.com/somus/termon/security/advisories/new)
for `somus/termon`.

Include:

- affected behavior or file path
- steps to reproduce
- expected impact
- any suggested fix or mitigation

Do not include raw provider keys, tokens, private repository contents, or other
sensitive data in the report unless GitHub Security Advisories requires it for
reproduction.

## Scope

Relevant security areas include:

- SSH authentication, host keys, and PROXY protocol trust
- player identity, rate limiting, and admission controls
- SQLite persistence, backups, restores, and migrations
- container and Dokploy deployment configuration
- PostHog telemetry filtering and credential handling
- content-pack validation and player-authored input handling
