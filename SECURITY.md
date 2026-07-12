# Security Policy

## Supported versions

This repository is an educational and portfolio reference implementation. Only
the latest commit on `main` is maintained.

## Reporting a vulnerability

Please do not open a public issue for a vulnerability that could create risk for
users who deploy a fork. Prefer GitHub's **Report a vulnerability** private
reporting form on the repository Security tab. If private reporting is not
available, use the security contact listed on the maintainer's GitHub profile.

Include:

- affected component and commit;
- preconditions;
- reproducible proof of concept in an isolated environment;
- impact;
- suggested remediation, if known.

## Scope and safe testing

Testing is authorised only against a copy you own or an environment where you
have explicit permission. Do not test third-party systems using this project.

## Important limitations

SecureLedger is **not** certified banking software. The default adapter stores
state in memory, and the development authentication mechanism trusts local
headers. Review `docs/security-assumptions.md` before running it.
