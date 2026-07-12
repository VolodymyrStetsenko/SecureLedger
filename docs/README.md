# Documentation index

| Document | Purpose |
|---|---|
| [Getting started](getting-started.md) | Install, run and verify both adapters |
| [Architecture](architecture.md) | Components, boundaries and consistency model |
| [Data model](data-model.md) | Accounts, journal, idempotency, audit and outbox schema |
| [PostgreSQL design](postgresql-design.md) | Transactions, locking, retry and reconciliation |
| [Testing](testing.md) | Local and CI evidence for each critical property |
| [Operations](operations.md) | Health, shutdown, reconciliation and incident procedures |
| [Threat model](threat-model.md) | Assets, abuse cases, controls and residual risk |
| [Security controls](security-controls.md) | Implemented control inventory and external references |
| [Security assumptions](security-assumptions.md) | Safe deployment scope and unimplemented controls |
| [Review checklist](review-checklist.md) | Repeatable review questions before merging a change |
| [Architecture decisions](adr/) | Accepted design decisions and consequences |

The OpenAPI contract is maintained separately at
[`api/openapi.yaml`](../api/openapi.yaml). Security reports follow
[`SECURITY.md`](../SECURITY.md).
