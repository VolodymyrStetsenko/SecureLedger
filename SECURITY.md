# Security policy

## Supported version

Only the latest commit on `main` receives security fixes.

## Report a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private
**Report a vulnerability** form on the repository Security tab. If private
reporting is unavailable, use the security contact on the maintainer's GitHub
profile.

Include enough information to reproduce and assess the report:

- affected commit and component;
- required preconditions and configuration;
- isolated reproduction steps or test;
- observed and expected behaviour;
- confidentiality, integrity or availability impact;
- a remediation idea, if available.

Do not include real credentials, customer data or third-party system details.

## Safe testing

Test only a local copy you control or an environment where you have explicit
authorisation. Do not scan or probe third-party services using this project.

## Response expectations

Reports are triaged by reproducibility, affected security boundary and impact.
A fix should include a regression test and updates to the threat model or an ADR
when the underlying design changes. Disclosure timing is coordinated after a
supported fix is available.

## Deployment warning

SecureLedger is not certified banking or payment-processing software. The
included identity headers are not authentication, and the local Compose
credentials do not provide production database privilege separation. Do not
expose the supplied configuration to an untrusted network or use it for real
funds. Review [security assumptions](docs/security-assumptions.md) before
execution.
