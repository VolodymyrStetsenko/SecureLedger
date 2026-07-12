# Security assumptions and deployment warning

## Safe execution scope

The supplied configuration is intended for local development, isolated code
review and authorised testing. Bindings in `compose.yaml` use `127.0.0.1` to
avoid accidental exposure on other host interfaces.

Do not place this service on the public internet or use it to hold, transfer or
represent real customer funds.

## Identity

`X-Principal-ID` and `X-Principal-Role` are asserted development headers. They
exercise authorisation policy but provide no authentication: any caller can
forge either value.

A production identity boundary would need, at minimum:

- TLS from a trusted ingress;
- signature validation for an accepted token algorithm;
- issuer, audience, expiry and not-before validation;
- key rotation and failure behaviour;
- separate human and workload identities;
- tenant-aware subject mapping;
- protection against a client bypassing the trusted ingress.

## Persistence and database authority

The default adapter is volatile memory. PostgreSQL mode is durable and enforces
ledger constraints, but the local Compose user owns broad database privileges.
That credential model is for reproducibility, not production separation.

A deployed service should not own its schema. Migration and runtime roles
should be distinct, runtime permissions should be minimal, and backups should
be encrypted, monitored and restoration-tested.

## Monetary representation

Amounts use signed 64-bit integer minor units. A value of `2500` means 25.00
only for a currency whose exponent is two. The validator accepts three uppercase
ASCII letters but does not maintain an ISO 4217 exponent registry, exchange
rates, cash rounding rules or currency lifecycle.

The API rejects negative/zero transfer amounts and checks arithmetic boundaries.
It does not implement fees, holds, reversals, chargebacks or foreign exchange.

## Journal and reconciliation

PostgreSQL triggers make journal transactions, postings, transfer intents and
audit records append-only for ordinary writes. A schema owner can still disable
or replace those controls. The reconciliation command detects a difference
between balances and postings but intentionally does not auto-repair it.

Production evidence needs access controls, retention policy, monitored exports
and a tamper-evident or independently administered destination.

## Risk events

PostgreSQL mode stores risk events in the same transaction as a qualifying
transfer. The worker retries delivery and recovers stale claims. The included
publisher writes a structured log event; it is not an AML, sanctions or fraud
decision system and is not a substitute for a durable external event service.

Memory mode uses a bounded process-local queue and can lose events on shutdown
or when the queue is full.

## Availability and operations

The HTTP server has finite header, read, write and idle timeouts plus a body
limit. The repository uses bounded database retries. The project does not yet
include rate limiting, autoscaling, multi-region failover, backup automation,
service-level objectives, metrics or distributed tracing.

`/healthz` proves that the process is serving HTTP. `/readyz` also pings the
configured repository. Neither endpoint proves that every downstream external
publisher is available.

## Assurance

Tests, CodeQL, database constraints and documented threat analysis provide
engineering evidence, not certification. No independent penetration test,
formal verification, regulatory assessment or external security approval is
claimed.
