# Security control inventory

## Purpose

This document maps implemented engineering controls to the evidence in this
repository. It is a review aid, not a claim of compliance with OWASP ASVS, NIST
SSDF or any financial regulation.

## Control matrix

| Objective | Implemented control | Evidence | Remaining work |
|---|---|---|---|
| Prevent unauthorised debit | Role validation and source ownership check | `internal/app/service.go`, negative service/HTTP tests | Replace asserted headers with authenticated identity and tenant policy |
| Prevent duplicate transfer | Mandatory actor-scoped key, immutable intent comparison, unique DB constraint | service, both adapters, replay integration test | Retention and tenant-scoping policy |
| Preserve accounting equation | Domain zero-sum validation and deferred PostgreSQL constraint | domain tests, schema negative test | Extend rules for additional transaction types |
| Prevent partial commit | Domain-specific atomic repository operation and one DB transaction | repository contract, integration lifecycle test | Fault-injection around database/network boundaries |
| Prevent double spend | Mutex or serializable row-locked transaction with retry | concurrency tests for both adapters | Load/abort telemetry and capacity testing |
| Preserve audit history | Audit row in financial transaction and append-only trigger | schema and repository tests | Separate DB roles and external tamper-evident export |
| Limit request attack surface | Strict media type/JSON, unknown-field rejection, one-object rule, 1 MiB ceiling | HTTP tests | Rate limits and authenticated ingress |
| Limit information disclosure | Stable public errors and no body/secret logging | HTTP mapper and middleware | Central log redaction/retention validation |
| Detect dependency outage | Repository-backed readiness endpoint | readiness tests | Metrics, alerts and dependency SLOs |
| Avoid risk-event loss | Transactional outbox, leases and retry | worker and PostgreSQL integration tests | Durable external sink and dead-letter policy |
| Detect ledger drift | Repeatable-read reconciliation with non-zero failure exit | reconciliation integration test | Scheduling, alerting and retained attestations |
| Reduce supply-chain risk | `go.sum`, versioned Actions, Dependabot, `govulncheck`, CodeQL and read-only CI permissions | workflows and dependency configuration | Immutable Action digests, signed releases, provenance and dependency review |
| Harden container runtime | Non-root distroless image, read-only root FS, dropped capabilities | `Dockerfile`, `compose.yaml` | Orchestrator policy, image signing and vulnerability scanning |

## Secure development references

The repository uses the following public sources as design references:

- [OWASP Application Security Verification Standard](https://owasp.org/www-project-application-security-verification-standard/)
  for systematic application-control review;
- [NIST Secure Software Development Framework, SP 800-218](https://csrc.nist.gov/pubs/sp/800/218/final)
  for secure development and software-integrity practices;
- [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html)
  for serializable retry semantics;
- [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html)
  for row-lock and deadlock behaviour;
- [Stripe API idempotent requests](https://docs.stripe.com/api/idempotent_requests)
  as a public example of binding retries to the original request.

No proprietary banking source code or private institutional system is included.
The implementation and documentation are specific to this repository.

## Review expectations

Security-sensitive changes should identify:

- the asset and trust boundary affected;
- success and failure invariants;
- negative and concurrent test evidence;
- migration and rollback behaviour;
- new log fields or sensitive data;
- residual risk and whether an ADR is required.

See [CONTRIBUTING.md](../CONTRIBUTING.md) and the
[review checklist](review-checklist.md).
